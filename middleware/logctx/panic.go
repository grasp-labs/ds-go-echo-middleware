package logctx

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// LogPanic logs a panic at ERROR, with the fields on ctx and a stack trace,
// and then lets the panic continue, so the program still crashes as before.
// Without it, Go prints the panic as plain text that JSON log filters and
// alerts cannot see.
//
// Defer it at the top of a goroutine or a worker cycle:
//
//	defer logctx.LogPanic(ctx)
//
// Defer LogPanic itself. Wrapping it, as in
// defer func() { logctx.LogPanic(ctx) }(), does not catch the panic, because
// Go's recover only works when called directly by the deferred function.
func LogPanic(ctx context.Context) {
	if r := recover(); r != nil {
		slog.ErrorContext(ctx, "panic",
			"error", fmt.Sprint(r),
			"stack", string(debug.Stack()),
		)
		panic(r)
	}
}
