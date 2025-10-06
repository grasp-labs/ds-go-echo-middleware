package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
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
func AuthenticationMiddleware(cfg interfaces.Config, logger interfaces.Logger, publicKeyPEM string, producer *adapters.ProducerAdapter, topic string) (echo.MiddlewareFunc, error) {
	// Parse the public key from PEM format
	publicKey, err := ParseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}

	// Helper to normalize token (strip "Bearer " if present)
	trimBearer := func(s string) string {
		const p = "Bearer "
		if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
			return s[len(p):]
		}
		return s
	}

	// Create and return the JWT middleware
	return middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		// Choose where you want to read the token:
		// - For Authorization header with optional "Bearer " prefix:
		KeyLookup: "header:Authorization",
		// - If you prefer X-Api-Key, change to: KeyLookup: "header:X-Api-Key"
		//   and set AuthScheme to "" (empty string) if the X-Api-Key header contains only the token.
		AuthScheme: "Bearer",

		Skipper: func(c echo.Context) bool {
			// Let CORS preflight pass
			if c.Request().Method == http.MethodOptions {
				return true
			}
			return false
		},

		Validator: func(raw string, c echo.Context) (bool, error) {
			// Store raw authorization header in the Echo context
			c.Set("Authorization", "Bearer "+raw)
			token := trimBearer(raw)

			if token == "" {
				logger.Error(c.Request().Context(), "Token is empty.")
				return false, WrapErr(c, "unauthorized")
			}

			claims := &models.Context{}
			parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					logger.Error(c.Request().Context(), "unexpected signing method: %v", t.Header["alg"])
					return false, WrapErr(c, "unauthorized")
				}
				return publicKey, nil
			})

			if err != nil || !parsed.Valid {
				logger.Error(c.Request().Context(), "Invalid token: %v", err)
				return false, WrapErr(c, "unauthorized")
			}

			// Stash claims in Echo context (typed key) and standard context
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
				return false, WrapErr(c, "unauthorized")
			}

			event := sdkmodels.EventJson{
				Id:          requestID,
				TenantId:    tenantID,
				EventType:   "login.success", // Check this
				EventSource: cfg.Name(),
				Timestamp:   time.Now().UTC(),
				Payload: &map[string]any{
					"subject":     claims.Sub,
					"path":        c.Path(),
					"user_agent":  c.Request().UserAgent(),
					"remote_addr": c.Request().RemoteAddr,
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), topic, event)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send auth success event to Kafka for target %s: %v", event.Id.String(), kafkaErr)
			}
			return true, nil
		},
		ErrorHandler: func(handlerErr error, c echo.Context) error {
			logger.Error(c.Request().Context(), "Jwt error: %v", handlerErr)

			requestIDStr := requestctx.GetRequestID(c.Request().Context())
			requestID, parseErr := uuid.Parse(requestIDStr)
			if parseErr != nil {
				logger.Error(c.Request().Context(), "Invalid request_id from context: %v", parseErr)
				requestID = uuid.New()
			}

			event := sdkmodels.EventJson{
				Id:          requestID,
				TenantId:    uuid.UUID{},
				EventType:   "login.failure", // Check this
				EventSource: cfg.Name(),
				Timestamp:   time.Now().UTC(),
				Payload: &map[string]interface{}{
					"subject":     "",
					"path":        c.Path(),
					"user_agent":  c.Request().UserAgent(),
					"remote_addr": c.Request().RemoteAddr,
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), event.Id.String(), event)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send auth failure event to Kafka for target %s: %v", event.Id.String(), kafkaErr)
			}

			return WrapErr(c, "unauthorized")
		},
	}), nil
}
