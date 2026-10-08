# ds-go-echo-middleware

![Build](https://github.com/grasp-labs/ds-go-echo-middleware/actions/workflows/ci.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/grasp-labs/ds-go-echo-middleware)](https://goreportcard.com/report/github.com/grasp-labs/ds-go-echo-middleware)
[![codecov](https://codecov.io/gh/grasp-labs/ds-go-echo-middleware/branch/main/graph/badge.svg)](https://codecov.io/gh/grasp-labs/ds-go-echo-middleware)
[![GitHub release](https://img.shields.io/github/v/release/grasp-labs/ds-go-echo-middleware)](https://github.com/grasp-labs/ds-go-echo-middleware/releases)
![License](https://img.shields.io/github/license/grasp-labs/ds-go-echo-middleware?cacheSeconds=60)

Reusable middleware components for Go applications using Echo — including structured authentication, authorization, request auditing, and request ID propagation, with support for Kafka-based event logging and cache-based permission lookups.

## Installation & Usage

See [docs/middleware-guide.md](./docs/middleware-guide.md) for how to use Middleware.

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

- Skips billing usage when a handler returns an error or an HTTP status of 400 or higher.
- Emits usage entry logs to Kafka for the purpose of billing usage. Captures RequestID, TenantID, used Memory and Time. Support X-OwnerID header for cost allocation.

### 📤 Kafka Integration

- Sends structured authentication/authorization events to a Kafka topic.
- Fully decoupled via an interface-based producer abstraction.

See [docs/kafka-integration.md](./docs/kafka-integration.md) for example of how to integrate with data streaming platform.

### ✅ Interfaces

This project uses clean interfaces for dependency injection:

Logger: structured logging with context support
Producer: Kafka producer interface
Config: provides ProductID, Permission, APICache, etc.
You can plug in your own implementations or mocks.

See [docs/interface-example-config.md](./docs/interface-example-config.md) for example of satisfying the config interface.

## 📦 Dependencies

- Echo – Web framework
- BigCache – In-memory cache
- Kafka Go – Kafka integration
- golang-jwt – JWT parsing/validation

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
