package fakes

import (
	"context"
	"fmt"
)

type MockLogger struct {
	infoCalled    bool
	warningCalled bool
	errorCalled   bool
	lastMsg       string
}

func (l *MockLogger) Info(ctx context.Context, format string, args ...any) {
	l.infoCalled = true
	l.lastMsg = fmt.Sprintf(format, args...)
}

func (l *MockLogger) Warning(ctx context.Context, format string, args ...any) {
	l.warningCalled = true
	l.lastMsg = fmt.Sprintf(format, args...)
}

func (l *MockLogger) Error(ctx context.Context, format string, args ...any) {
	l.errorCalled = true
	l.lastMsg = fmt.Sprintf(format, args...)
}
