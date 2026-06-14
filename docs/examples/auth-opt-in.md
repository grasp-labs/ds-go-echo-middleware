# 🔐 Example: Authentication opt-in features

 The authentication middleware keeps the same call signature: calling it with the
 original five arguments preserves the default behaviour (static PEM verification, no
 audience check). However, `Config.Issuer()` is now required so the middleware can
 bind tokens to the correct issuer (and derive JWKS URIs when enabled).

| Feature | How to enable | Default when omitted |
| ------- | ------------- | -------------------- |
| Key-rotation-safe verification (JWKS by `kid`) | `middleware.WithJWKS()` | Static PEM (the `publicKeyPEM` argument) |
| Audience-confusion defence (RFC 8707) | `middleware.WithAudience(resourceID)` (+ `middleware.WithSharedAudience(host)`) | `aud` value is not checked |
| RFC 9728 discovery endpoint + 401 challenge | `middleware.RegisterProtectedResource(...)` | No `/.well-known` route; 401s still carry a bare `Bearer` challenge |

> Always-on regardless of options: `iss` is enforced against `Config.Issuer()`,
> `cls` must be `user`/`app`, `exp`/`nbf` get a small clock-skew leeway, and 401s
> carry a `WWW-Authenticate` header.

---

## 1. Baseline (unchanged) — static PEM, no audience

```go
authMW, err := middleware.AuthenticationMiddleware(
	cfg, logger, publicKeyPEM, kafka.Producer(), kafka.ComplianceTopicID(),
)
if err != nil {
	log.Fatal(err)
}
e.Use(authMW)
```

---

## 2. Opt in to key rotation (JWKS by `kid`)

No PEM needed — the JWKS URI is derived from `Config.Issuer()` as
`{Issuer}/oauth/.well-known/jwks.json`. A signing-key rotation on the IdP is then
picked up at runtime (no redeploy).

```go
authMW, err := middleware.AuthenticationMiddleware(
	cfg, logger, "" /* publicKeyPEM ignored */, kafka.Producer(), kafka.ComplianceTopicID(),
	middleware.WithJWKS(),
)
```

---

## 3. Opt in to audience enforcement

When enabled, the check is a **set-membership** test: a token is accepted if its
`aud` contains **either** this service's resource id **or** the mesh-wide shared
audience. That single rule lets default/mesh tokens work everywhere while still
allowing a client to narrow a token to one API.

- `WithAudience(resourceID)` — your service's own id (`AUTH_RESOURCE_ID`). Also
  drives the `resource_metadata` value in the 401 challenge.
- `WithSharedAudience(host)` — the mesh-wide audience (`AUTH_SHARED_AUDIENCE`,
  e.g. `https://grasp-daas.com`) so default tokens reach you.

Gate it behind your own config flag during migration (turn on only once clients
request your resource id and the IdP allowlists it):

```go
const (
	resourceID      = "https://grasp-daas.com/api/state/v1"
	sharedAudience  = "https://grasp-daas.com"
)

opts := []middleware.AuthOption{middleware.WithJWKS()}

if cfg.AudienceEnforced() { // your own env flag, e.g. AUTH_AUDIENCE_REQUIRED
	opts = append(opts,
		middleware.WithAudience(resourceID),
		middleware.WithSharedAudience(sharedAudience),
	)
}

authMW, err := middleware.AuthenticationMiddleware(
	cfg, logger, "", kafka.Producer(), kafka.ComplianceTopicID(), opts...,
)
```

> Omit `WithSharedAudience` only if you want to reject mesh-wide tokens and
> require a token narrowed to exactly your resource id.
>
> Per-route override: apply a second `AuthenticationMiddleware` configured with
> `WithAudience(...)` to a sensitive route group even if the global switch is off.

---

## 4. Opt in to RFC 9728 discovery (PRM endpoint + challenge)

Call once on the root echo instance, **before** the auth chain, so the metadata
route stays public:

```go
const resourceID = "https://grasp-daas.com/api/your-service/v1"

middleware.RegisterProtectedResource(e, cfg.PathPrefix(), middleware.ResourceMetadata{
	Resource:             resourceID,
	AuthorizationServers: []string{cfg.Issuer()},
	ScopesSupported:      []string{"read", "write"},
})

// Then the protected chain:
e.Use(authMW)
```

This serves `GET {prefix}/.well-known/oauth-protected-resource` and makes every
401 carry `WWW-Authenticate: Bearer resource_metadata="…"`.

---

## 5. Reading the principal in a handler

After authentication, a normalized principal is available on the request
`context.Context` — framework-agnostic, so service/repo layers can read it too:

```go
import "github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"

func handler(c echo.Context) error {
	p, ok := requestctx.GetPrincipal(c.Request().Context())
	if !ok {
		return echo.ErrUnauthorized
	}

	// p.Kind is "user" or "app"; branch only for human-only / machine-only routes.
	switch p.Kind {
	case requestctx.KindUser:
		// p.ID is the user's email
	case requestctx.KindApp:
		// p.ID is the app's client_id
	}

	// p.TenantID (uuid), p.Roles ([]string, advisory), p.JTI (uuid) also available.
	_ = p.TenantID
	return c.NoContent(http.StatusOK)
}
```

---

## Full wiring (all opt-ins together)

```go
func setupAuth(e *echo.Echo, cfg config.Config, logger interfaces.Logger, kafka KafkaDeps) error {
	const resourceID = "https://grasp-daas.com/api/your-service/v1"

	// Public discovery surface (must precede the auth chain).
	middleware.RegisterProtectedResource(e, cfg.PathPrefix(), middleware.ResourceMetadata{
		Resource:             resourceID,
		AuthorizationServers: []string{cfg.Issuer()},
		ScopesSupported:      []string{"read", "write"},
	})

	opts := []middleware.AuthOption{middleware.WithJWKS()}
	if cfg.AudienceEnforced() {
		opts = append(opts,
			middleware.WithAudience(resourceID),
			middleware.WithSharedAudience("https://grasp-daas.com"),
		)
	}

	authMW, err := middleware.AuthenticationMiddleware(
		cfg, logger, "", kafka.Producer(), kafka.ComplianceTopicID(), opts...,
	)
	if err != nil {
		return err
	}
	e.Use(authMW)
	return nil
}
```
