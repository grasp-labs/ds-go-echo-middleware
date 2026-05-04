package middleware

import (
	"context"
	"encoding/json"
	"time"

	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-commonmodels/v3/commonmodels/entitlement"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/utils"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/requestctx"
)

// AuthorizationMiddleware for asserting a user is permitted
// to perform action.
func AuthorizationMiddleware(cfg interfaces.Config, logger interfaces.Logger, roles []string, url string, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			claims := &models.Context{}

			// Get userContext from Echo context
			userContext := c.Get("userContext")
			if userContext == nil {
				return errorHandler(c, &cfg, http.StatusUnauthorized, "User context not found", nil, logger, producer, "authz.denied", claims, topic)
			}

			claims, ok := userContext.(*models.Context)
			if !ok {
				return errorHandler(c, &cfg, http.StatusUnauthorized, "Invalid user context type", nil, logger, producer, "authz.denied", claims, topic)
			}

			// Get token from Echo Context set by Authorization middleware
			authorization := c.Get("Authorization")
			// Safely assert the value to a string
			authToken, ok := authorization.(string)
			if !ok {
				return errorHandler(c, &cfg, http.StatusUnauthorized, "Failed to assert authorization as string", nil, logger, producer, "authz.denied", claims, topic)

			}

			userID := claims.Sub

			entry, err := cfg.APICache().Get(userID)
			if err == nil {
				logger.Info(ctx, "Cache entry for user: %s", userID)
				if isAdmin(ctx, logger, entry) || hasAccess(ctx, logger, entry, roles) {
					logger.Info(ctx, "Entitlement accepts request for user: %s", userID)
					return next(c)
				}
			}

			// Make external entitlement API call
			startTime := time.Now().UTC()
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return errorHandler(c, &cfg, http.StatusInternalServerError, "Failed to create request to entitlement API", err, logger, producer, "authz.error", claims, topic)
			}
			req.Header.Set("Authorization", authToken)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return errorHandler(c, &cfg, http.StatusInternalServerError, "Failed to make request to Entitlement API", err, logger, producer, "authz.error", claims, topic)
			}

			defer func() { _ = resp.Body.Close() }()

			latency := time.Since(startTime)
			logger.Info(ctx, "Entitlement API latency ms: %s", latency.Milliseconds())

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return errorHandler(c, &cfg, http.StatusInternalServerError, "Failed to read response body from Entitlements API", err, logger, producer, "authz.error", claims, topic)
			}

			if resp.StatusCode != http.StatusOK {
				return errorHandler(c, &cfg, http.StatusUnauthorized, "Entitlements refused request", nil, logger, producer, "authz.denied", claims, topic)
			}

			err = cfg.APICache().Set(userID, body)
			if err != nil {
				logger.Error(ctx, "failed to set %s in cache", userID)
			}

			if !isAdmin(ctx, logger, body) && !hasAccess(ctx, logger, body, roles) {
				return errorHandler(c, &cfg, http.StatusForbidden, "Permission denied", nil, logger, producer, "authz.denied", claims, topic)
			}

			logger.Info(ctx, "Entitlement accepts request for user: %s", userID)
			return next(c)
		}
	}
}

// isAdmin returns true when the entitlements response contains the "admin" group.
func isAdmin(ctx context.Context, logger interfaces.Logger, responseBody []byte) bool {
	var groups []entitlement.Entitlement
	if err := json.Unmarshal(responseBody, &groups); err != nil {
		logger.Error(ctx, "Failed to unmarshal entitlements response: %v", err)
		return false
	}
	for _, g := range groups {
		if g.Name == "users.admins" {
			logger.Info(ctx, "Admin group membership grants access")
			return true
		}
	}
	return false
}

// hasAccess returns true when the entitlements response contains any of the required roles.
func hasAccess(ctx context.Context, logger interfaces.Logger, responseBody []byte, roles []string) bool {
	var groups []entitlement.Entitlement
	if err := json.Unmarshal(responseBody, &groups); err != nil {
		logger.Error(ctx, "Failed to unmarshal entitlements response: %v", err)
		return false
	}
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	for _, g := range groups {
		if allowed[g.Name] {
			logger.Info(ctx, "Match found for name: %s", g.Name)
			return true
		}
	}
	return false
}

func errorHandler(
	c echo.Context,
	cfg *interfaces.Config,
	status_code int,
	errMessage string,
	err error,
	logger interfaces.Logger,
	producer *adapters.ProducerAdapter,
	eventType string,
	claims *models.Context,
	topic string,
) error {
	req := c.Request()
	ctx := req.Context()

	if err != nil {
		logger.Error(ctx, "%s: %v", errMessage, err)
	} else {
		logger.Error(ctx, "%s", errMessage)
	}

	// Parse (or generate) request ID set byt RequestID middleware
	requestID := requestctx.GetOrNewRequestUUID(ctx)
	sessionID := requestctx.GetOrNewSessionUUID(ctx)

	tenantID, err := claims.GetTenantId()
	if err != nil {
		tenantID = uuid.UUID{}
	}

	// Optional message from header
	var message *string
	if val := req.Header.Get("X-Message"); val != "" {
		message = &val
	}

	event := sdkmodels.EventJson{
		Id:          uuid.New(),
		TenantId:    tenantID,
		RequestId:   requestID,
		SessionId:   sessionID,
		EventType:   eventType,
		EventSource: utils.CreateServicePrincipleID(*cfg),
		Timestamp:   time.Now().UTC(),
		Message:     message,
		Payload: &map[string]any{
			"status_code": status_code,
			"subject":     claims.Sub,
			"error":       safeErr(err),
			"path":        c.Path(),
			"user_agent":  req.UserAgent(),
			"remote_addr": req.RemoteAddr,
		},
	}

	kafkaErr := producer.Send(ctx, topic, event)
	if kafkaErr != nil {
		logger.Error(ctx, "Failed to send %s event to Kafka topic '%s' for event ID %s: %v", eventType, topic, event.Id.String(), kafkaErr)
	}

	return echo.ErrForbidden
}

func safeErr(err error) *string {
	var e string
	if err != nil {
		e = err.Error()
		return &e
	}
	return nil
}
