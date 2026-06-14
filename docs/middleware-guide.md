# Middleware guide

How to install and use the `ds-go-echo-middleware` package in another Go project.

## ✅ 1. Install via go get

This module is published at major version **v3** (the path includes `/v3`):

```bash
go get github.com/grasp-labs/ds-go-echo-middleware/v3@v3.0.0
```

This will:

- Add the dependency to your `go.mod`
- Download the module from GitHub
- Lock to the specified version

> **Upgrading from v2?** v3 is a breaking change: the `interfaces.Config`
> interface gained an `Issuer()` method (see step 3). Update your import paths
> to `.../v3/...` and add `Issuer()` to your config implementation. The new
> JWKS / audience features are opt-in and don't change existing behaviour — see
> [`examples/auth-opt-in.md`](./examples/auth-opt-in.md).

## ✅ 2. Import and Use in Your Project

In your Echo project:

```go
import (
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware"
)

e := echo.New()

// Build the authentication middleware (returns an error if the key/issuer is invalid).
authMW, err := middleware.AuthenticationMiddleware(cfg, logger, publicKeyPEM, producer, complianceTopic)
if err != nil {
	log.Fatal(err)
}

// Apply middleware
e.Use(middleware.RequestIDMiddleware(logger))
e.Use(authMW)
e.Use(middleware.AuthorizationMiddleware(cfg, logger, roles, entitlementURL, producer, complianceTopic))
e.Use(middleware.AuditMiddleware(cfg, logger, producer, auditTopic))
```

### Current middleware signatures

```go
func RequestIDMiddleware(logger interfaces.Logger) echo.MiddlewareFunc
func AuthenticationMiddleware(cfg interfaces.Config, logger interfaces.Logger, publicKeyPEM string, producer *adapters.ProducerAdapter, topic string, opts ...AuthOption) (echo.MiddlewareFunc, error)
func AuthorizationMiddleware(cfg interfaces.Config, logger interfaces.Logger, roles []string, url string, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc
func UsageMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc
func LocaleMiddleware(def string) echo.MiddlewareFunc
func APIKeyMiddleware(logger interfaces.Logger, validKeys []string) (echo.MiddlewareFunc, error)
```

## ✅ 3. Interfaces you must implement

- `interfaces.Logger` — logging behavior
- `interfaces.Config` — service config values (incl. `Issuer()`, see below)
- `interfaces.Producer` — emit Kafka/audit events

`interfaces.Config` requires an `Issuer()` accessor returning **this
environment's identity-server issuer URL** (the IdP — same for every service):

| Env | `Issuer()` |
| --- | ---------- |
| prod | `https://auth.grasp-daas.com` |
| dev | `https://auth-dev.grasp-daas.com` |
| local | `http://localhost:8000` |

The authentication middleware enforces the token `iss` against it and, when
JWKS is enabled, derives the JWKS URI as `{Issuer}/oauth/.well-known/jwks.json`.
See [`examples/interface-example-config.md`](./examples/interface-example-config.md)
for a full `Config` implementation.

## ✅ 4. Opt-in: key rotation, audience, discovery

The authentication middleware is backward compatible; new capabilities are
opt-in via functional options and one wiring call:

- `middleware.WithJWKS()` — rotation-safe verification (resolve keys by `kid`
  from the live JWKS instead of a static PEM).
- `middleware.WithAudience(resourceID)` — RFC 8707 audience-confusion defence.
- `middleware.RegisterProtectedResource(e, prefix, meta)` — RFC 9728 discovery
  endpoint + `WWW-Authenticate` challenge.

Full examples: [`examples/auth-opt-in.md`](./examples/auth-opt-in.md).

## 🧪 Optional: Local Replace for Development

If you're working on the middleware locally and want to test it in another project without publishing a release:

```go
// In your consuming project's go.mod:
replace github.com/grasp-labs/ds-go-echo-middleware/v3 => ../ds-go-echo-middleware
```

This will make Go use your local copy instead of pulling from GitHub.
