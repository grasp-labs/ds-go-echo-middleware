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
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

type Entitlement struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TenantId string `json:"tenant_id"`
}

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
				return errorHandler(c, http.StatusUnauthorized, "User context not found", nil, logger, producer, "authz.denied", claims, topic)
			}

			claims, ok := userContext.(*models.Context)
			if !ok {
				return errorHandler(c, http.StatusUnauthorized, "Invalid user context type", nil, logger, producer, "authz.denied", claims, topic)
			}

			// Get token from Echo Context set by Authorization middleware
			authorization := c.Get("Authorization")
			// Safely assert the value to a string
			authToken, ok := authorization.(string)
			if !ok {
				return errorHandler(c, http.StatusUnauthorized, "Failed to assert authorization as string", nil, logger, producer, "authz.denied", claims, topic)

			}

			// Use the user information from the claims (e.g., Sub or Rol)
			userID := claims.Sub

			entry, err := cfg.APICache().Get(userID)
			if err == nil {
				logger.Info(ctx, "Cache entry for user: %s", userID)
				if userIsMember(ctx, logger, entry, roles) {
					logger.Info(ctx, "Entitlement accepts request for user: %s", userID)
					return next(c)
				}
			}

			// Make external entitlement API call
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return errorHandler(c, http.StatusInternalServerError, "Failed to create request to entitlement API", err, logger, producer, "authz.error", claims, topic)
			}
			req.Header.Set("Authorization", authToken)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return errorHandler(c, http.StatusInternalServerError, "Failed to make request to Entitlement API", err, logger, producer, "authz.error", claims, topic)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return errorHandler(c, http.StatusInternalServerError, "Failed to read response body from Entitlements API", err, logger, producer, "authz.error", claims, topic)
			}

			if resp.StatusCode != http.StatusOK {
				return errorHandler(c, http.StatusUnauthorized, "Entitlements refused request", nil, logger, producer, "authz.denied", claims, topic)
			}

			// Cache result
			err = cfg.APICache().Set(userID, body)
			if err != nil {
				logger.Error(ctx, "failed to set %s in cache", userID)
			}

			if !userIsMember(ctx, logger, body, roles) {
				return errorHandler(c, http.StatusForbidden, "Permission denied", nil, logger, producer, "authz.denied", claims, topic)
			}

			logger.Info(ctx, "Entitlement accepts request for user: %s", userID)
			return next(c)
		}
	}
}

// Function asserting if target group is one of the groups
// user has a membership in.
func userIsMember(ctx context.Context, logger interfaces.Logger, responseBody []byte, namesToMatch []string) bool {
	var entitlements []Entitlement

	// Unmarshal the JSON response into a slice of ApiResponse
	err := json.Unmarshal(responseBody, &entitlements)
	if err != nil {
		logger.Error(ctx, "Failed to unmarshal API response: %v", err)
		return false
	}

	// Create a map for quick lookup of names to match
	nameSet := make(map[string]bool)
	for _, name := range namesToMatch {
		nameSet[name] = true
	}

	// Check if any of the names match in the response
	for _, item := range entitlements {
		if _, exists := nameSet[item.Name]; exists {
			logger.Info(ctx, "Match found for name: %s", item.Name)
			return true // Return true as soon as a match is found
		}
	}

	// Return false if no match was found
	return false
}

func errorHandler(
	c echo.Context,
	status int,
	message string,
	err error,
	logger interfaces.Logger,
	producer *adapters.ProducerAdapter,
	eventType string,
	claims *models.Context,
	topic string,
) error {
	if err != nil {
		logger.Error(c.Request().Context(), "%s: %v", message, err)
	} else {
		logger.Error(c.Request().Context(), "%s", message)
	}

	// Parse (or generate) request ID set byt RequestID middleware
	requestID := requestctx.GetOrNewRequestUUID(c.Request().Context())
	sessionID := requestctx.GetOrNewSessionUUID(c.Request().Context())

	tenantID, err := claims.GetTenantId()
	if err != nil {
		tenantID = uuid.UUID{}
	}

	event := sdkmodels.EventJson{
		Id:          uuid.New(),
		TenantId:    tenantID,
		RequestId:   requestID,
		SessionId:   sessionID,
		EventType:   eventType,
		EventSource: "",
		Timestamp:   time.Now().UTC(),
		Payload: &map[string]any{
			"subject":     claims.Sub,
			"error":       message,
			"path":        c.Path(),
			"user_agent":  c.Request().UserAgent(),
			"remote_addr": c.Request().RemoteAddr,
		},
	}

	kafkaErr := producer.Send(c.Request().Context(), topic, event)
	if kafkaErr != nil {
		logger.Error(c.Request().Context(), "Failed to send %s event to Kafka topic '%s' for event ID %s: %v", eventType, topic, event.Id.String(), kafkaErr)
	}

	return echo.ErrForbidden
}
