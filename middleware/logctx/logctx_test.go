package logctx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/testlog"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
	"github.com/stretchr/testify/require"
)

func TestRequestContext(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output)
	ctx := requestctx.SetRequestID(t.Context(), "upstream-request")
	ctx = requestctx.SetPrincipal(ctx, requestctx.Principal{
		Kind:     requestctx.KindUser,
		TenantID: uuid.MustParse("89B9C527-E093-4B1D-99D7-04ABC5DE73DD"),
		ID:       "user@example.com",
	})
	logger.ErrorContext(ctx, "update failed", "error", errors.New("first line\nsecond line"))

	testlog.Equal(t, `[{
		"level": "ERROR",
		"msg": "update failed",
		"request_id": "upstream-request",
		"cls": "user",
		"tenant_id": "89b9c527-e093-4b1d-99d7-04abc5de73dd",
		"principal_id": "user@example.com",
		"error": "first line\nsecond line"
	}]`, testlog.Read(t, &output))
}

func TestNewHandlerRespectsJSONOptions(t *testing.T) {
	var output bytes.Buffer
	var level slog.LevelVar
	logger := slog.New(NewHandler(slog.NewJSONHandler(&output, &slog.HandlerOptions{
		Level:     &level,
		AddSource: true,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == "principal_id" {
				return slog.String(attr.Key, "redacted")
			}
			return attr
		},
	})))
	ctx := requestctx.SetRequestID(t.Context(), "request-1")
	ctx = requestctx.SetPrincipal(ctx, requestctx.Principal{ID: "user@example.com"})
	logger.DebugContext(ctx, "disabled")
	level.Set(slog.LevelDebug)
	logger.With("component", "worker").DebugContext(ctx, "working")

	entries := testlog.Read(t, &output)
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, "DEBUG", entry["level"])
	require.Equal(t, "working", entry["msg"])
	require.Equal(t, "worker", entry["component"])
	require.Equal(t, "request-1", entry["request_id"])
	require.Equal(t, "redacted", entry["principal_id"])
	require.NotEmpty(t, entry["source"])
}

func TestNewHandlerSupportsTextOutput(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&output, nil))).
		With("component", "worker").WithGroup("details")
	ctx := requestctx.SetRequestID(t.Context(), "request-1")
	logger.InfoContext(ctx, "working", "attempt", 2)

	for _, field := range []string{
		"level=INFO", "msg=working", "component=worker",
		"details.attempt=2", "details.request_id=request-1",
	} {
		require.Contains(t, output.String(), field)
	}
}

func TestMissingContext(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output)
	logger.Info("startup")
	ctx := requestctx.SetUserContext(t.Context(), &claims.Context{
		Cls: requestctx.KindUser,
		Rsc: "89b9c527-e093-4b1d-99d7-04abc5de73dd:tenant",
		Sub: "legacy@example.com",
	})
	logger.InfoContext(ctx, "no identity")
	logger.DebugContext(t.Context(), "disabled")

	testlog.Equal(t, `[
		{"level": "INFO", "msg": "startup"},
		{"level": "INFO", "msg": "no identity"}
	]`, testlog.Read(t, &output))
}

func TestJobRunContext(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output)
	parent := requestctx.SetRequestID(t.Context(), "request-1")
	first := WithJobRun(parent, "expiry_sweep")
	second := WithJobRun(parent, "expiry_sweep")
	logger.InfoContext(first, "first run")
	logger.InfoContext(second, "second run")
	logger.InfoContext(first, "first run again")
	logger.InfoContext(parent, "parent")

	records := testlog.Read(t, &output)
	require.Len(t, records, 4)
	for _, record := range records {
		require.Equal(t, "request-1", record["request_id"])
	}
	for _, record := range records[:3] {
		require.Equal(t, "expiry_sweep", record["job_name"])
		require.NoError(t, uuid.Validate(record["run_id"].(string)))
	}
	require.NotEqual(t, records[0]["run_id"], records[1]["run_id"])
	require.Equal(t, records[0]["run_id"], records[2]["run_id"])
	require.NotContains(t, records[3], "run_id")
	require.NotContains(t, records[3], "job_name")
}

func TestWithAttrsAndGroupRetainContext(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output).With("component", "worker").WithGroup("details").With("attempt", 2)
	ctx := requestctx.SetRequestID(t.Context(), "request-1")
	logger.InfoContext(ctx, "working", "count", 3)

	testlog.Equal(t, `[{
		"level": "INFO",
		"msg": "working",
		"component": "worker",
		"details": {"attempt": 2, "count": 3, "request_id": "request-1"}
	}]`, testlog.Read(t, &output))
}

func TestConcurrentRequestContext(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			id := fmt.Sprintf("request-%d", i)
			ctx := requestctx.SetRequestID(t.Context(), id)
			logger.With("worker", i).InfoContext(ctx, id)
		})
	}
	wg.Wait()

	records := testlog.Read(t, &output)
	require.Len(t, records, 32)
	seen := make(map[string]bool)
	for _, record := range records {
		id := record["request_id"].(string)
		require.Equal(t, record["msg"], id)
		require.False(t, seen[id])
		seen[id] = true
	}
}

func TestAdapter(t *testing.T) {
	var output bytes.Buffer
	var adapter interfaces.Logger = Adapter{Logger: New(&output)}
	ctx := requestctx.SetRequestID(t.Context(), "request-1")
	adapter.Info(ctx, "attempt %d", 1)
	adapter.Warning(ctx, "attempt %d", 2)
	adapter.Error(ctx, "attempt %d", 3)

	testlog.Equal(t, `[
		{"level": "INFO", "msg": "attempt 1", "request_id": "request-1"},
		{"level": "WARN", "msg": "attempt 2", "request_id": "request-1"},
		{"level": "ERROR", "msg": "attempt 3", "request_id": "request-1"}
	]`, testlog.Read(t, &output))
}

func TestNewLeavesGlobalLoggersUnchanged(t *testing.T) {
	defaultSlog := slog.Default()
	defaultWriter := log.Writer()
	New(io.Discard).Info("local logger")
	require.Same(t, defaultSlog, slog.Default())
	require.Equal(t, defaultWriter, log.Writer())
}

func TestContextDoesNotDuplicateExplicitPrincipal(t *testing.T) {
	var output bytes.Buffer
	ctx := requestctx.SetPrincipal(t.Context(), requestctx.Principal{
		Kind:     requestctx.KindApp,
		ID:       "app-client",
		TenantID: uuid.MustParse("89b9c527-e093-4b1d-99d7-04abc5de73dd"),
	})
	New(&output).InfoContext(ctx, "iam: denied", "principal_id", "app-client", "action", "state:read")
	require.Equal(t, 1, strings.Count(output.String(), `"principal_id":`))
	testlog.Equal(t, `[{
		"level": "INFO",
		"msg": "iam: denied",
		"cls": "app",
		"principal_id": "app-client",
		"tenant_id": "89b9c527-e093-4b1d-99d7-04abc5de73dd",
		"action": "state:read"
	}]`, testlog.Read(t, &output))
}
