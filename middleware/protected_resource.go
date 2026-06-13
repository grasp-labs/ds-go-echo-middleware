package middleware

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// WellKnownProtectedResourcePath is the RFC 9728 metadata path, served under the
// service's path prefix.
const WellKnownProtectedResourcePath = "/.well-known/oauth-protected-resource"

// ResourceMetadata is the per-service identity advertised by the PRM document.
// Resource MUST be byte-for-byte identical to the `aud` the IdP mints and the
// service's OAUTH_RESOURCE_ALLOWLIST entry.
type ResourceMetadata struct {
	Resource             string   // e.g. "https://grasp-daas.com/api/ai-gateway/v1"
	AuthorizationServers []string // e.g. ["https://auth.grasp-daas.com"]
	ScopesSupported      []string // advisory; e.g. ["read","write"]
}

// RegisterProtectedResource wires a service's RFC 9728 discovery surface:
//
//  1. Serves GET {prefix}{WellKnownProtectedResourcePath} (public, no auth,
//     CORS "*", Cache-Control "public, max-age=3600") returning the PRM doc.
//  2. Wraps e.HTTPErrorHandler so every 401 carries
//     WWW-Authenticate: Bearer resource_metadata="{Resource}{WellKnownProtectedResourcePath}".
//
// Call once, on the root echo instance, before routes are served. The metadata
// route must NOT be behind the auth chain.
func RegisterProtectedResource(e *echo.Echo, prefix string, meta ResourceMetadata) {
	e.GET(prefix+WellKnownProtectedResourcePath, protectedResourceHandler(meta))

	metadataURL := meta.Resource + WellKnownProtectedResourcePath
	challenge := fmt.Sprintf("Bearer resource_metadata=%q", metadataURL)
	base := e.HTTPErrorHandler // capture (default or already-wrapped)
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if he, ok := err.(*echo.HTTPError); ok &&
			he.Code == http.StatusUnauthorized && !c.Response().Committed {
			c.Response().Header().Set(echo.HeaderWWWAuthenticate, challenge)
		}
		base(err, c)
	}
}

func protectedResourceHandler(meta ResourceMetadata) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderAccessControlAllowOrigin, "*")
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		return c.JSON(http.StatusOK, map[string]any{
			"resource":                 meta.Resource,
			"authorization_servers":    meta.AuthorizationServers,
			"bearer_methods_supported": []string{"header"},
			"scopes_supported":         meta.ScopesSupported,
		})
	}
}
