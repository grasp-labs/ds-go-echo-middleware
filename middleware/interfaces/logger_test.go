package interfaces_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
)

type mockLogger struct {
	called  bool
	lastMsg string
}

func (l *mockLogger) Info(ctx context.Context, format string, args ...any) {
	l.called = true
	l.lastMsg = format
}

func (l *mockLogger) Warning(ctx context.Context, format string, args ...any) {
	l.called = true
	l.lastMsg = format
}
func (l *mockLogger) Error(ctx context.Context, format string, args ...any) {
	l.called = true
	l.lastMsg = format
}

func TestLogger_InfoCanBeCalled(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	// Confirm it satisfies the interface at compile time
	var _ interfaces.Logger = logger

	logger.Info(ctx, "test message")
	assert.True(t, logger.called)
	assert.Equal(t, "test message", logger.lastMsg)
}

func TestLogger_WarningCanBeCalled(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	// Confirm it satisfies the interface at compile time
	var _ interfaces.Logger = logger

	logger.Warning(ctx, "test message")
	assert.True(t, logger.called)
	assert.Equal(t, "test message", logger.lastMsg)
}

func TestLogger_ErrorCanBeCalled(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	// Confirm it satisfies the interface at compile time
	var _ interfaces.Logger = logger

	logger.Error(ctx, "test message")
	assert.True(t, logger.called)
	assert.Equal(t, "test message", logger.lastMsg)
}
