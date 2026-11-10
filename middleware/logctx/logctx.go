// Package logctx enriches slog handlers with request and job context.
package logctx

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
)

// New returns an INFO-level JSON logger with request and job context.
// It does not change slog.Default.
func New(output io.Writer) *slog.Logger {
	return slog.New(NewHandler(slog.NewJSONHandler(output, nil)))
}

// NewHandler adds request, principal, and job context to records handled by next.
// The supplied handler controls output format, level, and attribute formatting.
func NewHandler(next slog.Handler) slog.Handler {
	return contextHandler{Handler: next}
}

type jobRunKey struct{}

type jobRun struct {
	name string
	id   string
}

// WithJobRun attaches a job name and a fresh run ID to ctx.
func WithJobRun(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, jobRunKey{}, jobRun{
		name: name,
		id:   uuid.NewString(),
	})
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	record = record.Clone()
	if ctx != nil {
		if run, ok := ctx.Value(jobRunKey{}).(jobRun); ok {
			addContextAttr(&record, slog.String("job_name", run.name))
			addContextAttr(&record, slog.String("run_id", run.id))
		}
		if requestID := requestctx.GetRequestID(ctx); requestID != "" {
			addContextAttr(&record, slog.String("request_id", requestID))
		}
		if principal, ok := requestctx.GetPrincipal(ctx); ok {
			if principal.Kind != "" {
				addContextAttr(&record, slog.String("cls", principal.Kind))
			}
			if principal.TenantID != uuid.Nil {
				addContextAttr(&record, slog.String("tenant_id", principal.TenantID.String()))
			}
			if principal.ID != "" {
				addContextAttr(&record, slog.String("principal_id", principal.ID))
			}
		}
	}
	return h.Handler.Handle(ctx, record)
}

// Explicit per-call fields take precedence over context fields.
func addContextAttr(record *slog.Record, attr slog.Attr) {
	exists := false
	record.Attrs(func(existing slog.Attr) bool {
		exists = existing.Key == attr.Key
		return !exists
	})
	if !exists {
		record.AddAttrs(attr)
	}
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}

// Adapter sends existing printf-style middleware logs to Logger.
type Adapter struct {
	Logger *slog.Logger
}

func (a Adapter) Info(ctx context.Context, format string, args ...any) {
	a.Logger.InfoContext(ctx, fmt.Sprintf(format, args...))
}

func (a Adapter) Warning(ctx context.Context, format string, args ...any) {
	a.Logger.WarnContext(ctx, fmt.Sprintf(format, args...))
}

func (a Adapter) Error(ctx context.Context, format string, args ...any) {
	a.Logger.ErrorContext(ctx, fmt.Sprintf(format, args...))
}
