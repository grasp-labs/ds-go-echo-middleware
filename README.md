# ds-go-echo-middleware

Reusable middleware components for Go applications using Echo — including structured authentication, authorization, request auditing, and request ID propagation, with support for Kafka-based event logging and cache-based permission lookups.

## 🚀 Features

### 🧠 Request ID Middleware

- Injects a UUIDv4 X-Request-ID into headers and request context if not present.
- Makes request tracing easy across services and middleware. 
- Should be the first middleware in the stack to run.

### 🔑 JWT Authentication Middleware

- Validates RS256-signed JWT tokens using a configurable RSA public key.
- Extracts user claims into structured context (userContext) for downstream access.

### 🔐 Authorization Middleware

- Verifies user entitlements based on roles from cache or external API.
- Supports role matching, caching with BigCache, and structured audit logging.

### 🧾 Audit Middleware

- Emits detailed request/response audit logs to Kafka.
- Captures request ID, user identity, headers, and payload (for mutating methods).

### 🧾 Usage Middleware

- Emits usage entry logs to Kafka for the purpose of billing usage. Captures RequestID, TenantID, used Memory and Time. Support X-OwnerID header for cost allocation.

### 📤 Kafka Integration

- Sends structured authentication/authorization events to a Kafka topic.
- Fully decoupled via an interface-based producer abstraction.

## Usage

1. Wire into your Echo app:

```go
e := echo.New()

// Set up logger, config, and Kafka producer
logger := NewLogger()
producer := NewKafkaProducer(...)
cfg := NewAppConfig(...)

e.Use(middleware.RequestIDMiddleware(logger))
e.Use(middleware.AuditMiddleware(cfg, logger, producer))

authMiddleware, _ := middleware.AuthenticationMiddleware(logger, publicKeyPEM, producer)
e.Use(authMiddleware)

e.Use(middleware.AuthorizationMiddleware(cfg, logger, producer))
```

2. Access user context in handlers:

```go
func handler(c echo.Context) error {
    claims := c.Get("userContext").(*models.Context)
    return c.String(http.StatusOK, "Hello "+claims.Sub)
}
```

### ✅ Interfaces

This project uses clean interfaces for dependency injection:

Logger: structured logging with context support
Producer: Kafka producer interface
Config: provides ProductID, Permission, APICache, etc.
You can plug in your own implementations or mocks.

## 📦 Dependencies

- Echo – Web framework
- BigCache – In-memory cache
- Kafka Go – Kafka integration
- golang-jwt – JWT parsing/validation

## 📌 Versioning

This project follows Semantic Versioning 2.0.0 to indicate release stability and compatibility:

MAJOR.MINOR.PATCH
MAJOR – breaking changes
MINOR – new features, backward compatible
PATCH – bug fixes, small improvements

### 🏷️ Tagging a Release

To cut a new release:

```bash
# 1. Commit all changes
git commit -am "Prepare release v1.0.0"

# 2. Create a version tag
git tag v1.0.0

# 3. Push the tag to origin
git push origin v1.0.0
```

To dry run locally:

```bash
# Install if needed
brew install goreleaser

# Run dry-run release
goreleaser release --snapshot --skip-publish --rm-dist

# Local release - assumes GITHUB_TOKEN is defined as environment variable
goreleaser release --clean --config .goreleaser.yml
```

## Tests

### Unit test

To execute all tests recursively, call

```bash
go test ./... 
```


To execute test in single directory, call

```bash
go test ./folder/subfolder
```

Add verbosity by flag `-v`
