package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/utils"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
)

type authConfig struct {
	audience string // "" = disabled
	useJWKS  bool   // true = resolve keys by kid from live JWKS
}

// AuthOption configures AuthenticationMiddleware.
type AuthOption func(*authConfig)

// WithAudience enables the RFC 8707 audience-confusion defence: a verified token
// is rejected unless its `aud` contains resource. Pass the service's exact
// resource id (== ResourceMetadata.Resource). Omit to disable (default).
func WithAudience(resource string) AuthOption {
	return func(a *authConfig) { a.audience = resource }
}

// WithJWKS enables key-rotation-safe verification: instead of pinning a single
// static PEM, the verifying key is resolved by the token's `kid` from the live
// JWKS at {Config.Issuer()}/oauth/.well-known/jwks.json, cached with a short TTL
// and refreshed on an unknown kid (per the key-rotation contract). When set, the
// publicKeyPEM argument is ignored and may be empty.
func WithJWKS() AuthOption {
	return func(a *authConfig) { a.useJWKS = true }
}

// jwksWellKnownSuffix is appended to the issuer to locate the JWKS document.
const jwksWellKnownSuffix = "/oauth/.well-known/jwks.json"

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

// AuthenticationMiddleware returns the JWT middleware configured with a validator.
// Pass WithAudience to enable RFC 8707 audience binding; existing callers with
// five arguments compile unchanged and retain today's behaviour.
func AuthenticationMiddleware(cfg interfaces.Config, logger interfaces.Logger, publicKeyPEM string, producer *adapters.ProducerAdapter, topic string, opts ...AuthOption) (echo.MiddlewareFunc, error) {
	ac := &authConfig{}
	for _, o := range opts {
		o(ac)
	}

	issuer := strings.TrimRight(cfg.Issuer(), "/")
	if issuer == "" {
		return nil, errors.New("config issuer is empty; cannot enforce iss")
	}

	// Resolve the verifying key source: either the live JWKS (rotation-safe,
	// keyed by kid) or a single static PEM (legacy / fixed-key deployments).
	var (
		staticKey *rsa.PublicKey
		jwks      *jwksCache
	)
	if ac.useJWKS {
		jwks = newJWKSCache(issuer + jwksWellKnownSuffix)
	} else {
		var err error
		staticKey, err = ParseRSAPublicKey(publicKeyPEM)
		if err != nil {
			return nil, err
		}
	}

	// keyFunc resolves the RSA public key for a parsed token, rejecting any
	// non-RSA signing method (never accept alg:none / HS*).
	keyFunc := func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		if jwks != nil {
			kid, _ := t.Header["kid"].(string)
			return jwks.getKey(kid)
		}
		return staticKey, nil
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
			parsed, err := jwt.ParseWithClaims(token, claims, keyFunc)

			if err != nil || !parsed.Valid {
				logger.Error(c.Request().Context(), "Invalid token: %v", err)
				return false, WrapErr(c, "unauthorized")
			}

			// Enforce issuer against this environment's configured issuer.
			if claims.Iss != issuer {
				logger.Error(c.Request().Context(), "token iss %q does not match expected issuer %q", claims.Iss, issuer)
				return false, WrapErr(c, "unauthorized")
			}

			// Reject any unrecognized principal kind (cls must be user|app).
			if !requestctx.ValidKind(claims.Cls) {
				logger.Error(c.Request().Context(), "token has invalid cls: %q", claims.Cls)
				return false, WrapErr(c, "unauthorized")
			}

			// Audience check (RFC 8707) — only when enabled via WithAudience.
			if ac.audience != "" && !slices.Contains(claims.Aud, ac.audience) {
				logger.Error(c.Request().Context(), "token aud %v missing resource %s", claims.Aud, ac.audience)
				return false, WrapErr(c, "unauthorized")
			}

			// Build the normalized principal (kind/id/tenant/roles/jti).
			principal, err := requestctx.NewPrincipal(claims)
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid tenant_id from claims: %s", claims.Rsc)
				return false, WrapErr(c, "unauthorized")
			}

			// Stash claims in Echo context (typed key) and standard context
			c.Set("userContext", claims)

			// Also inject into context.Context so it propagates downstream
			// to functions not tied to echo such as Kafka.
			ctx := requestctx.SetUserContext(c.Request().Context(), claims)
			ctx = requestctx.SetPrincipal(ctx, principal)
			c.SetRequest(c.Request().WithContext(ctx))

			// Should send Login Succeeded event
			requestID := requestctx.GetOrNewRequestUUID(c.Request().Context())
			sessionID := requestctx.GetOrNewSessionUUID(c.Request().Context())

			// Optional message from header
			var message *string
			if val := c.Request().Header.Get("X-Message"); val != "" {
				message = &val
			}

			event := sdkmodels.EventJson{
				Id:          uuid.New(),
				TenantId:    principal.TenantID,
				RequestId:   requestID,
				SessionId:   sessionID,
				EventType:   "login.success", // Check this
				EventSource: utils.CreateServicePrincipleID(cfg),
				Timestamp:   time.Now().UTC(),
				Message:     message,
				Payload: &map[string]any{
					"subject":     claims.Sub,
					"cls":         principal.Kind,
					"jti":         claims.Jti.String(),
					"tenant_id":   principal.TenantID.String(),
					"path":        c.Path(),
					"user_agent":  c.Request().UserAgent(),
					"remote_addr": c.Request().RemoteAddr,
				},
			}

			sendEventAsync(producer, logger, topic, event, "login.success")
			return true, nil
		},
		ErrorHandler: func(handlerErr error, c echo.Context) error {
			logger.Error(c.Request().Context(), "Jwt error: %v", handlerErr)

			// Attach the RFC 6750 challenge to the 401. When this service has a
			// resource id (WithAudience), point at its PRM document; otherwise a
			// bare Bearer challenge.
			if !c.Response().Committed {
				challenge := "Bearer"
				if ac.audience != "" {
					challenge = fmt.Sprintf("Bearer resource_metadata=%q", ac.audience+WellKnownProtectedResourcePath)
				}
				c.Response().Header().Set(echo.HeaderWWWAuthenticate, challenge)
			}

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
				EventSource: utils.CreateServicePrincipleID(cfg),
				Timestamp:   time.Now().UTC(),
				Payload: &map[string]any{
					"subject":     "",
					"path":        c.Path(),
					"user_agent":  c.Request().UserAgent(),
					"remote_addr": c.Request().RemoteAddr,
				},
			}

			sendEventAsync(producer, logger, topic, event, "login.failure")

			return echo.ErrUnauthorized
		},
	}), nil
}
