# Contract: OAuth Protected Resource discovery in `ds-go-echo-middleware`

**Status:** proposed — to be implemented in `github.com/grasp-labs/ds-go-echo-middleware/v2`.
**Consumers:** every Grasp EKS API service (`ai-gateway`, `state`, `file`, `config`, …).
**Implements:** RFC 9728 (Protected Resource Metadata), RFC 6750 §3 (the `WWW-Authenticate`
challenge), RFC 8707 (audience binding / audience-confusion defence).

This is the framework half of the IdP's "RFC 9728 — Protected Resource Metadata"
reference. The IdP already publishes the authorization-server half (RFC 8414 +
JWKS + RFC 7591 DCR). Each resource server must publish *its* half: a metadata
document, a 401 challenge that points at it, and an `aud` check. Because that
half is **identical** across services, it belongs in the shared middleware, not
copied into each service. Services supply only their identity (resource id,
issuer, scopes) and opt in.

---

## 1. Why shared, not per-service

The three pieces below are byte-for-byte the same logic for every service:

1. **PRM document** — `GET {prefix}/.well-known/oauth-protected-resource` returning
   a small JSON doc (resource id + authorization server). Only the field *values*
   differ per service.
2. **401 challenge** — every unauthenticated/invalid-token response must carry
   `WWW-Authenticate: Bearer resource_metadata="…"`. The producer of the 401 is
   already the shared `AuthenticationMiddleware` (it returns `echo.ErrUnauthorized`).
3. **`aud` check** — reject any token whose `aud` does not contain this service's
   resource id. The shared `AuthenticationMiddleware` already parses the claims
   (`claims.Context.Aud []string`) and verifies signature + `iss`
   (`Context.Valid()`), but **does not** check `aud` today. That gap is in the
   shared layer, so the fix belongs there — fixing it once gives every service
   the audience-confusion defence.

Putting these in the middleware means a service that adopts the new version gets
the full discovery surface with ~5 lines of wiring.

---

## 2. Current shared-middleware behaviour (baseline)

`middleware.AuthenticationMiddleware(cfg, logger, publicKeyPEM, producer, topic)`:

- Reads `Authorization: Bearer …`, parses with `jwt.ParseWithClaims` into
  `claims.Context`, verifies **RS256** against a static PEM public key.
- `claims.Context.Valid()` checks `exp/nbf/iat`, `iss ∈ {auth, auth-dev}`, `sub`,
  and that `rsc` is `tenantId:tenantName`.
- **Does not** check `aud`.
- On any failure the `KeyAuthConfig.ErrorHandler` emits a `login.failure` event and
  returns `echo.ErrUnauthorized`, so the 401 flows through Echo's
  `HTTPErrorHandler`. No `WWW-Authenticate` header is set.

The contract additions below must preserve all of the above.

---

## 3. API to add (exact surface)

### 3a. Protected Resource Metadata + 401 challenge — new file `middleware/protected_resource.go`

```go
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
```

### 3b. Audience check — extend `AuthenticationMiddleware` (backward-compatible)

Add a variadic option so existing callers (every current service + the template)
keep compiling unchanged. Default behaviour is identical to today.

```go
type authConfig struct {
	audience string // "" = disabled
}

// AuthOption configures AuthenticationMiddleware.
type AuthOption func(*authConfig)

// WithAudience enables the RFC 8707 audience-confusion defence: a verified token
// is rejected unless its `aud` contains resource. Pass the service's exact
// resource id (== ResourceMetadata.Resource). Omit to disable (default).
func WithAudience(resource string) AuthOption {
	return func(a *authConfig) { a.audience = resource }
}

// Signature becomes (append the variadic — source-compatible):
//   func AuthenticationMiddleware(cfg interfaces.Config, logger interfaces.Logger,
//       publicKeyPEM string, producer *adapters.ProducerAdapter, topic string,
//       opts ...AuthOption) (echo.MiddlewareFunc, error)
```

Inside the `Validator`, **after** signature + claims validation succeed and
before returning `true`, add:

```go
if ac.audience != "" && !slices.Contains(claims.Aud, ac.audience) {
	logger.Error(c.Request().Context(), "token aud %v missing resource %s", claims.Aud, ac.audience)
	return false, WrapErr(c, "unauthorized")
}
```

This routes through the existing `ErrorHandler` → `echo.ErrUnauthorized`, so the
challenge header from 3a is attached automatically. No separate middleware needed.

---

## 4. Behaviour spec (acceptance criteria)

**PRM endpoint** — `GET {prefix}/.well-known/oauth-protected-resource`:
- 200, `Content-Type: application/json`.
- Public: never behind the auth chain; no token required.
- Headers `Access-Control-Allow-Origin: *` and `Cache-Control: public, max-age=3600`.
- Body exactly: `resource`, `authorization_servers` (array), `bearer_methods_supported: ["header"]`, `scopes_supported`.
- `resource` equals the configured resource id verbatim.

**401 challenge** — any 401 from the protected chain:
- Carries `WWW-Authenticate: Bearer resource_metadata="{resource}/.well-known/oauth-protected-resource"`.
- Applies to both missing token and invalid token (the shared middleware collapses
  both to `echo.ErrUnauthorized`; the plain `resource_metadata` form is valid for
  both — the RFC 6750 `error="invalid_token"` variant is optional and not required
  by this contract).
- Non-401 responses (404, 403, 5xx, 200) are untouched.
- Header is set only when the response is not yet committed.

**`aud` check** (only when `WithAudience` is set):
- A token whose `aud` does not contain the resource id → 401 (with challenge).
- A token whose `aud` contains the resource id (among others) → passes.
- When `WithAudience` is not set → behaviour identical to today (no `aud` check).

**Backward compatibility:**
- `AuthenticationMiddleware` called with the old 5 args compiles and behaves exactly as before.
- `RegisterProtectedResource` is additive; wrapping `HTTPErrorHandler` must call the captured prior handler so existing error behaviour is preserved.

---

## 5. Per-service wiring (what each service provides)

Each service computes its identity and makes two calls. For `ai-gateway` the
identity is the public host joined with the path prefix:

| Env  | `Resource` (== `aud`)                          | Issuer                            |
| ---- | ---------------------------------------------- | --------------------------------- |
| prod | `https://grasp-daas.com/api/ai-gateway/v1`     | `https://auth.grasp-daas.com`     |
| dev  | `https://grasp-daas.com/api/ai-gateway-dev/v1` | `https://auth-dev.grasp-daas.com` |

The service supplies a `ResourceID()` accessor (`"https://grasp-daas.com" + PathPrefix()`),
its issuer (`DownStream().IDPBaseURL()`), and an enforcement flag
(`AudienceEnforced()`, default false from env `AUDIENCE_ENFORCEMENT`). Wiring in
`server.go` (mirrors what `ds-go-echo-template` should adopt):

```go
mw.RegisterProtectedResource(e, cfg.PathPrefix(), mw.ResourceMetadata{
	Resource:             cfg.ResourceID(),
	AuthorizationServers: []string{cfg.DownStream().IDPBaseURL()},
	ScopesSupported:      []string{"read", "write"},
})

var authOpts []mw.AuthOption
if cfg.AudienceEnforced() {
	authOpts = append(authOpts, mw.WithAudience(cfg.ResourceID()))
}
auth, err := mw.AuthenticationMiddleware(
	cfg, logger, cfg.JwtKey(), kafka.Producer(), kafka.ComplianceTopicID(), authOpts...,
)
```

> The `AudienceEnforced` gate exists because turning on the `aud` reject before
> the IdP mints the bound `aud` would reject every caller. Sequence: (1) add the
> resource to `OAUTH_RESOURCE_ALLOWLIST` on the IdP; (2) confirm clients send
> `resource=<resource id>` on the token request; (3) verify real tokens carry the
> resource in `aud`; (4) flip `AUDIENCE_ENFORCEMENT=true`.

---

## 6. Tests to add in the middleware repo

- PRM handler: 200, exact JSON fields, CORS + Cache-Control headers, no auth required.
- `RegisterProtectedResource`: a route returning `echo.ErrUnauthorized` yields a 401
  with the expected `WWW-Authenticate` value; a non-401 (e.g. `echo.ErrNotFound`) has
  no such header; a prior custom `HTTPErrorHandler` is still invoked.
- `WithAudience`: matching `aud` passes; missing/mismatched `aud` → 401; option absent → no check.
- Existing `AuthenticationMiddleware` tests still pass unchanged (signature compat).

---

## 7. Rollout

1. Implement 3a + 3b in `ds-go-echo-middleware`, add §6 tests, cut **v2.4.0**.
2. Bump `ai-gateway` (and the template) to v2.4.0 and add the §5 wiring +
   `ResourceID()` / `AudienceEnforced()` config accessors.
3. Coordinate the IdP `OAUTH_RESOURCE_ALLOWLIST` entry per env, then flip
   `AUDIENCE_ENFORCEMENT` on once tokens verify.
