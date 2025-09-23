package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

// ParseRSAPublicKey parses a PEM-encoded RSA public key and handles PKCS8 or PKCS1 formats
func ParseRSAPublicKey(pemKey string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing public key")
	}

	// Try parsing PKCS8 public key
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		if rsaPubKey, ok := pubKey.(*rsa.PublicKey); ok {
			return rsaPubKey, nil
		}
		return nil, errors.New("parsed key is not an RSA public key")
	}

	// Try parsing PKCS1 public key
	rsaPubKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA public key: %w", err)
	}

	return rsaPubKey, nil
}

// AuthenticationMiddleware returns the JWT middleware configured with a validator
func AuthenticationMiddleware(cfg interfaces.Config, logger interfaces.Logger, publicKeyPEM string, producer *adapters.ProducerAdapter) (echo.MiddlewareFunc, error) {
	// Parse the public key from PEM format
	publicKey, err := ParseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}

	// Create and return the JWT middleware
	return middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		Validator: func(token string, c echo.Context) (bool, error) {

			// Store authorization in the Echo context
			c.Set("Authorization", "Bearer "+token)

			if token == "" {
				logger.Error(c.Request().Context(), "Token is none.")
				// Return a 401 Unauthorized response when the token is missing
				return false, echo.NewHTTPError(http.StatusUnauthorized, "Unauthorized.")
			}

			claims := &models.Context{}
			parsedToken, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
				// Ensure the signing method is RS256
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					logger.Error(c.Request().Context(), "unexpected signing method: %v", t.Header["alg"])
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return publicKey, nil
			})

			// Check if parsing and validation were successful
			if err != nil || !parsedToken.Valid {
				logger.Error(c.Request().Context(), "Invalid token: %v", err)
				return false, echo.NewHTTPError(http.StatusUnauthorized, "Unauthorized.")
			}

			// Store the claims struct in the Echo context for handler usage
			c.Set("userContext", claims)

			// Also inject into context.Context so it propagates downstream
			// to functions not tied to echo such as Kafka.
			ctx := requestctx.SetUserContext(c.Request().Context(), claims)
			c.SetRequest(c.Request().WithContext(ctx))

			// Should send Login Succeded event
			requestIDStr := requestctx.GetRequestID(c.Request().Context())
			requestID, err := uuid.Parse(requestIDStr)
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid request_id from context: %v", err)
				requestID = uuid.New()
			}

			tenantID, err := claims.GetTenantId()
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid tenant_id from claims: %s", claims.Rsc)
				return false, echo.NewHTTPError(http.StatusUnauthorized, "Unauthorized.")
			}

			loginEvent := models.AuthEvent{
				ID:          requestID,
				ServiceName: cfg.Name(),
				Type:        "login.success",
				Subject:     claims.Sub,
				TenantID:    tenantID,
				Path:        c.Path(),
				UserAgent:   c.Request().UserAgent(),
				RemoteAddr:  c.Request().RemoteAddr,
				Timestamp:   time.Now().UTC(),
			}

			eventMap := sdkmodels.EventJson{
				Id:          loginEvent.ID,
				TenantId:    loginEvent.TenantID,
				EventType:   loginEvent.Type,
				EventSource: loginEvent.ServiceName,
				Timestamp:   loginEvent.Timestamp,
				Payload: &map[string]interface{}{
					"subject":     loginEvent.Subject,
					"path":        loginEvent.Path,
					"user_agent":  loginEvent.UserAgent,
					"remote_addr": loginEvent.RemoteAddr,
				},
			}

			producer.Send(c.Request().Context(), requestID.String(), eventMap)
			return true, nil
		},
		ErrorHandler: func(err error, c echo.Context) error {
			// Ensure the response is standardized with a 401 status
			logger.Error(c.Request().Context(), "Jwt error: %v", err)

			// Should send Login Failure event
			requestIDStr := requestctx.GetRequestID(c.Request().Context())
			requestID, err := uuid.Parse(requestIDStr)
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid request_id from context: %v", err)
				requestID = uuid.New()
			}

			loginEvent := models.AuthEvent{
				ID:         requestID,
				Type:       "login.failure",
				UserAgent:  c.Request().UserAgent(),
				RemoteAddr: c.Request().RemoteAddr,
				Path:       c.Path(),
				Error:      err.Error(),
				Timestamp:  time.Now().UTC(),
			}

			eventMap := sdkmodels.EventJson{
				Id:        loginEvent.ID,
				EventType: loginEvent.Type,
				Timestamp: loginEvent.Timestamp,
				Payload: &map[string]interface{}{
					"error":       loginEvent.Error,
					"path":        loginEvent.Path,
					"user_agent":  loginEvent.UserAgent,
					"remote_addr": loginEvent.RemoteAddr,
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), loginEvent.ID.String(), eventMap)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send auth failure event to Kafka for target %s: %v", loginEvent.ID.String(), kafkaErr)
			}

			return echo.NewHTTPError(http.StatusUnauthorized, "Unauthorized.")
		},
	}), nil
}
