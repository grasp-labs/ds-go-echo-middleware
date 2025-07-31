package middleware_test

import (
	"context"
	"fmt"

	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
)

type mockLogger struct {
	infoCalled    bool
	warningCalled bool
	errorCalled   bool
	lastMsg       string
}

func (l *mockLogger) Info(ctx context.Context, format string, args ...any) {
	l.infoCalled = true
	l.lastMsg = fmt.Sprintf(format, args...)
}

func (l *mockLogger) Warning(ctx context.Context, format string, args ...any) {
	l.warningCalled = true
	l.lastMsg = fmt.Sprintf(format, args...)
}

func (l *mockLogger) Error(ctx context.Context, format string, args ...any) {
	l.errorCalled = true
	l.lastMsg = fmt.Sprintf(format, args...)
}

// --- Mock Config ---
type mockConfig struct {
	name          string
	productID     uuid.UUID
	memoryLimitMB int16
	apiCache      *bigcache.BigCache
}

func (c *mockConfig) Name() string                 { return c.name }
func (c *mockConfig) ProductID() uuid.UUID         { return c.productID }
func (c *mockConfig) MemoryLimitMB() int16         { return c.memoryLimitMB }
func (c *mockConfig) APICache() *bigcache.BigCache { return c.apiCache }

// --- Mock Producer ---
type mockProducer struct {
	called bool
	key    string
	value  any
}

func (p *mockProducer) Send(ctx context.Context, key string, value any) error {
	p.called = true
	p.key = key
	p.value = value
	return nil
}

func (p *mockProducer) Close() error {
	return nil
}
