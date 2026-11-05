# Middleware guide

How to install and use the `ds-go-echo-middleware` package in another Go project.

## ✅ 1. Install via go get

Requires Go 1.27.3 or newer.

This module is published at major version **v3** (the path includes `/v3`):

```bash
go get github.com/grasp-labs/ds-go-echo-middleware/v3@<version>
```

Choose a published release containing the logging APIs below. This will:

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
	"log/slog"
	"os"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/logctx"
)

logger := logctx.New(os.Stdout)
slog.SetDefault(logger)
legacyLogger := logctx.Adapter{Logger: logger}

e := echo.New()

// Build the authentication middleware (returns an error if the key/issuer is invalid).
authMW, err := middleware.AuthenticationMiddleware(cfg, legacyLogger, publicKeyPEM, producer, complianceTopic)
if err != nil {
	log.Fatal(err)
}

// Apply middleware
e.Use(middleware.RequestIDMiddleware(legacyLogger))
e.Use(middleware.RequestLogger(logger))
e.Use(middleware.Recover(logger))
e.Use(authMW)
e.Use(middleware.AuthorizationMiddleware(cfg, legacyLogger, roles, entitlementURL, producer, complianceTopic))
e.Use(middleware.AuditMiddleware(cfg, legacyLogger, producer, auditTopic))
```

Register request IDs, logging, and recovery globally before CORS and route middleware
so public endpoints, missing routes, preflights, and rejected requests receive logs.

### Current middleware signatures

```go
func RequestIDMiddleware(logger interfaces.Logger) echo.MiddlewareFunc
func RequestLogger(logger *slog.Logger) echo.MiddlewareFunc
func Recover(logger *slog.Logger) echo.MiddlewareFunc
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
- `middleware.WithAudience(resourceID)` — RFC 8707 audience-confusion defence
  (this service's own resource id).
- `middleware.WithSharedAudience(host)` — additionally accept the mesh-wide
  shared audience (e.g. `https://grasp-daas.com`) so default tokens reach you.
  The audience check is a set-membership test: a token passes if its `aud`
  contains either value.
- `middleware.RegisterProtectedResource(e, prefix, meta)` — RFC 9728 discovery
  endpoint + `WWW-Authenticate` challenge.

Full examples: [`examples/auth-opt-in.md`](./examples/auth-opt-in.md).

## Logging

`logctx.New(output)` creates an INFO-level JSON logger. The adapter in the startup
example connects it to the existing printf-style middleware API. Pass the request
context to `InfoContext`, `WarnContext`, or `ErrorContext`. The logger adds
`request_id` from that context, so omit it from `logger.With(...)` and the log
call itself. When authentication supplies a
principal, logs also include `cls` (`user` or `app`), `principal_id`, and `tenant_id`
from `requestctx.GetPrincipal`.

```go
logger.InfoContext(c.Request().Context(), "item saved", "item_id", id)
```

`RequestLogger` records the final status, `duration_ms`, and full `uri`, including
query parameters. Statuses below 500 use INFO; 5xx use ERROR. `Recover` logs the
panic and stack before returning the error to the outer request logger.

For background work, create one context per run and pass it to logs and database
operations. It receives `job_name` and a fresh `run_id`:

```go
runCtx := logctx.WithJobRun(ctx, "expiry_sweep")
logger.InfoContext(runCtx, "expiry sweep started")
```

To configure format, level, or attributes, wrap a handler with `NewHandler`:

```go
handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
logger := slog.New(logctx.NewHandler(handler))
```

### GORM logging

Use `adapter/gormlog` from this module when opening the database:

```go
db, err := gorm.Open(dialector, &gorm.Config{Logger: gormlog.New(logger)})
```

Defaults log errors and queries slower than 200 ms, ignore record-not-found
errors, and hide bound parameters. To override them, modify GORM's native config:

```go
sqlConfig := gormlog.DefaultConfig()
sqlConfig.SlowThreshold = 500 * time.Millisecond
sqlLogger := gormlog.NewWithConfig(logger, sqlConfig)
```

`NewWithConfig` uses the complete config, including zero and `false` values.
The slog handler controls formatting; GORM's `Colorful` setting is unused.
Call either constructor before queries run. The first call replaces GORM's process-wide
recorder filter to hide `Scan` parameters regardless of `ParameterizedQueries`, which
controls ordinary queries. This does not change parameters sent to the database.
Use `db.WithContext(ctx)` so SQL records share request or job correlation fields.

## 🧪 Optional: Local Replace for Development

If you're working on the middleware locally and want to test it in another project without publishing a release:

```go
// In your consuming project's go.mod:
replace github.com/grasp-labs/ds-go-echo-middleware/v3 => ../ds-go-echo-middleware
```

This will make Go use your local copy instead of pulling from GitHub.
