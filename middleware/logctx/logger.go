package logctx

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/requestctx"
)

// defaultContext is used when user context is missing.
// Prevents nil dereference and provides default traceability fields.
var defaultContext = &claims.Context{
	Rsc: "00000000-0000-0000-0000-000000000000:unknown",
	Sub: "unknown",
}

type Logger struct{}

func (Logger) Info(ctx context.Context, format string, args ...any) {
	Info(ctx, format, args...)
}

func (Logger) Warning(ctx context.Context, format string, args ...any) {
	Warning(ctx, format, args...)
}

func (Logger) Error(ctx context.Context, format string, args ...any) {
	Error(ctx, format, args...)
}

func Info(c context.Context, format string, args ...any) {
	log.Printf("%s %s", buildLogPrefix("INFO", c), fmt.Sprintf(format, args...))
}

func Warning(c context.Context, format string, args ...any) {
	log.Printf("%s %s", buildLogPrefix("WARN", c), fmt.Sprintf(format, args...))
}

func Error(c context.Context, format string, args ...any) {
	log.Printf("%s %s", buildLogPrefix("ERROR", c), fmt.Sprintf(format, args...))
}

// Log Prefix
//
// To generate userContext aware log statement a call to `buildLogPrefix`
// is made as part of creating the `info`, `warning` and `error` log statements.
// User Context allows log lines to be tagged and then searchable on a user and
// tenant level.
func buildLogPrefix(level string, ctx context.Context) string {
	//var tenantIdStr, userId string
	// Parse (or generate) request ID set by RequestID middleware
	requestIDStr := requestctx.GetRequestID(ctx)
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		requestID = uuid.New()
	}

	userCtx := requestctx.GetUserContext(ctx)
	if userCtx == nil {
		userCtx = defaultContext
	}

	_, file, line, ok := runtime.Caller(2)
	caller := "unknown"
	if ok {
		caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	tenantID, err := userCtx.GetTenantId()
	if err != nil {
		tenantID = uuid.Nil
	}

	return fmt.Sprintf("[%s][%s][%s][%s][%s]", level, requestID, tenantID, userCtx.Sub, caller)
}
