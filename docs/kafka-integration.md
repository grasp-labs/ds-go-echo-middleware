# 📡 Kafka Integration

This middleware package is designed to support audit and authentication event publishing to Kafka using a pluggable `Producer` interface.

## ✅ Interface Recap

To integrate with Kafka, implement the following interface:

```go
type Producer interface {
	Send(ctx context.Context, key string, value any) error
	Close() error
}
```

## 📦 producer.go (Example implementation)

Full example of what a Kafka Producer might look like. Notice the `Send` and `Close` methods of the `Producer`struct.

```go
package kafka

import (
	"context"
	"encoding/json"

	"github.com/grasp-labs/ds-health/api/internal/logctx"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

// Create new Kafka producer.
//
// Parameters:
//   - ctx: Package context defines the Context type, which carries deadlines, cancellation signals,
//     and other request-scoped values across API boundaries and between processes.
//   - brokers: The list of brokers used to discover the partitions available on the kafka cluster.
//   - topic: The topic that the writer will produce messages to.
//
// Return Producer struct.
func NewProducer(c context.Context, brokers []string, topic string) *Producer {
	logctx.Info(c, "Configuring new producer for %s, %s", brokers, topic)
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      brokers,
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: -1,
	})

	return &Producer{writer: writer}
}

// Send writes a message to Kafka using the provided key and value.
//
// Parameters:
//   - ctx: the request-scoped context (used for cancellation or deadlines)
//   - key: used for Kafka partitioning; all messages with the same key are routed to the same partition
//   - value: any data structure to be serialized into JSON and sent as the Kafka message payload
//
// Returns an error if the message could not be marshaled or written to the Kafka topic.
func (p *Producer) Send(ctx context.Context, key string, value any) error {
	logctx.Info(ctx, "Sending message to topic %s with key %s.", p.writer.Topic, key)
	msgBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: msgBytes,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
```

## 🧪 Example Usage in Middleware

```go
auditProducer := kafka.NewProducer(ctx, cfg.Event.Brokers, cfg.Event.Topic.Audit)
```

## 🧾 Configuring the Kafka Topic

While not part of the interfaces.Config, Kafka config is typically modeled like this:

```go
type Config struct {
    Name  string
    Event struct {
        Brokers []string
        Topic   struct {
            Audit string
            Usage string
        }
    }
}
```

Example:

```go
cfg := &Config{
    Name: "Service #1",
    Event: EventConfig{
        Brokers: []string{"172.30.50.48:30090", "172.30.50.48:30091"},
        Topic: Topics{
            Audit: "ds.core.config.compliance.1.0.0",
            Usage: "ds.core.config.billing.1.0.0",
        },
    },
}
```

## 🧾 Sample Audit Event

Audit events are typically created within middleware and sent like this:

```go
...
entry := models.AuditEntry{
    ID:         requestID,
    TenantID:   userContext.GetTenantId(),
    Subject:    userContext.Sub,
    Jti:        userContext.Jti,
    HTTPMethod: request.Method,
    Resource:   deriveResource(c.Path()),
    Timestamp:  time.Now().UTC(),
    SourceIP:   request.RemoteAddr,
    UserAgent:  request.UserAgent(),
    Service:    cfg.Name(),
    Endpoint:   c.Path(),
    FullURL:    request.URL.String(),
    Payload:    payload,
}

kafkaErr := producer.Send(c.Request().Context(), entry.ID.String(), entry)
if kafkaErr != nil {
    logger.Error(c.Request().Context(), "Failed to send audit entry to Kafka for target %s: %v", entry.ID.String(), err)
}
```

## 🔐 Sample Authentication/Authorization Events

For login success/failure or access control, you might define:

```go
type AuthEvent struct {
	ID          uuid.UUID `json:"id"`
	ServiceName string    `json:"service_name"`
	Type        string    `json:"type"` // "login.success" or "login.failure"
	Subject     string    `json:"subject"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Error       string    `json:"error"`
	Path        string    `json:"path"`
	UserAgent   string    `json:"user_agent"`
	RemoteAddr  string    `json:"remote_addr"`
	Timestamp   time.Time `json:"timestamp"`
}
```

Emit with:

```go
producer.Send(ctx, requestID.String(), AuthEvent{...})
```

## ✅ Make note of

- Kafka messages are automatically marshaled to JSON in the Send method.
- Your structs (e.g., AuditEntry, AuthEvent) must have valid JSON tags.
- Errors are logged and fail gracefully to avoid disrupting main application flow.
