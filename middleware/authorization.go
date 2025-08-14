package middleware

import (
	"context"
	"encoding/json"
	"time"

	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
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
func AuthorizationMiddleware(cfg interfaces.Config, logger interfaces.Logger, roles []string, url string, producer interfaces.Producer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Get userContext from Echo context
			userContext := c.Get("userContext")
			if userContext == nil {
				return errorHandler(c, http.StatusUnauthorized, "User context not found", nil, logger, producer, "authz.denied", "")
			}

			claims, ok := userContext.(*models.Context)
			if !ok {
				return errorHandler(c, http.StatusUnauthorized, "Invalid user context type", nil, logger, producer, "authz.denied", "")
			}

			// Get token from Echo Context set by Authorization middleware
			authorization := c.Get("Authorization")
			// Safely assert the value to a string
			authToken, ok := authorization.(string)
			if !ok {
				return errorHandler(c, http.StatusUnauthorized, "Failed to assert authorization as string", nil, logger, producer, "authz.denied", claims.Sub)

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
				return errorHandler(c, http.StatusInternalServerError, "Failed to create request to entitlement API", err, logger, producer, "authz.error", claims.Sub)
			}
			req.Header.Set("Authorization", authToken)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return errorHandler(c, http.StatusInternalServerError, "Failed to make request to Entitlement API", err, logger, producer, "authz.error", claims.Sub)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return errorHandler(c, http.StatusInternalServerError, "Failed to read response body from Entitlements API", err, logger, producer, "authz.error", claims.Sub)
			}

			if resp.StatusCode != http.StatusOK {
				return errorHandler(c, http.StatusUnauthorized, "Entitlements refused request", nil, logger, producer, "authz.denied", claims.Sub)
			}

			// Cache result
			cfg.APICache().Set(userID, body)

			if !userIsMember(ctx, logger, body, roles) {
				return errorHandler(c, http.StatusForbidden, "Permission denied", nil, logger, producer, "authz.denied", claims.Sub)
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
	echoCtx echo.Context,
	status int,
	message string,
	err error,
	logger interfaces.Logger,
	producer interfaces.Producer,
	eventType string,
	subject string,
) error {
	if err != nil {
		logger.Error(echoCtx.Request().Context(), "%s: %v", message, err)
	} else {
		logger.Error(echoCtx.Request().Context(), "%s", message)
	}

	// Parse (or generate) request ID set byt RequestID middleware
	requestIDStr := requestctx.GetRequestID(echoCtx.Request().Context())
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		logger.Error(echoCtx.Request().Context(), "Invalid request_id from context: %v", err)
		requestID = uuid.New()
	}

	producer.Send(echoCtx.Request().Context(), subject, models.AuthEvent{
		ID:         requestID,
		Type:       eventType,
		Subject:    subject,
		Error:      message,
		Path:       echoCtx.Path(),
		UserAgent:  echoCtx.Request().UserAgent(),
		RemoteAddr: echoCtx.Request().RemoteAddr,
		Timestamp:  time.Now().UTC(),
	})

	return echoCtx.JSON(status, map[string]string{"error": message})
}
