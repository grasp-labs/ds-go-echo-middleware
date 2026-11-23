package logctx

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/testlog"
	"github.com/stretchr/testify/require"
)

func TestLogPanicLogsThenPanicsAgain(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(New(&output))
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := WithJobRun(t.Context(), "enrich")
	require.PanicsWithValue(t, "nil map", func() {
		defer LogPanic(ctx)
		panic("nil map")
	})

	records := testlog.Read(t, &output)
	require.Len(t, records, 1)
	require.Equal(t, "ERROR", records[0]["level"])
	require.Equal(t, "panic", records[0]["msg"])
	require.Equal(t, "nil map", records[0]["error"])
	require.Equal(t, "enrich", records[0]["job_name"])
	require.Contains(t, records[0]["stack"], "TestLogPanicLogsThenPanicsAgain")
}

func TestLogPanicDoesNothingWithoutPanic(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(New(&output))
	t.Cleanup(func() { slog.SetDefault(previous) })

	func() {
		defer LogPanic(context.Background())
	}()

	require.Empty(t, output.String())
}
