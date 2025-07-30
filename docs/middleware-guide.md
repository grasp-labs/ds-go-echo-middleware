# Middleware guide

To install and use the ds-go-echo-middleware package in another Go project:

## ✅ 1. Install via go get

Use the latest tagged release (e.g. v1.0.0):

```bash
go get github.com/grasp-labs/ds-go-echo-middleware@v1.0.0
```

This will:

- Add the dependency to your go.mod
- Download the module from GitHub
- Lock to the specified version

## ✅ 2. Import and Use in Your Project

In your Echo project:

```go
import (
	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
)

e := echo.New()

// Apply middleware
e.Use(middleware.RequestIDMiddleware(logger))
e.Use(middleware.AuthenticationMiddleware(logger, publicKey, producer))
e.Use(middleware.AuthorizationMiddleware(cfg, logger, producer))
e.Use(middleware.AuditMiddleware(cfg, logger, producer))
```

You must implement the following interfaces in your project:

- interfaces.Logger — for logging behavior
- interfaces.Config — to provide config values and permissions
- interfaces.Producer — to emit Kafka/audit events

## 🧪 Optional: Local Replace for Development

If you're working on the middleware locally and want to test it in another project without publishing a release:

```go
// In your consuming project's go.mod:
replace github.com/grasp-labs/ds-go-echo-middleware => ../ds-go-echo-middleware
```

This will make Go use your local copy instead of pulling from GitHub.
