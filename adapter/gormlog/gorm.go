// Package gormlog connects GORM SQL logging to slog.
package gormlog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

var configureRecorder = sync.OnceFunc(func() {
	// Scan uses GORM's process-wide recorder. Drop bound values before it formats the SQL.
	gormlogger.RecorderParamsFilter = func(_ context.Context, sql string, _ ...any) (string, []any) {
		return sql, nil
	}
})

// New returns a GORM logger with DefaultConfig. Call it before GORM queries run.
// See NewWithConfig for the process-wide Scan filter.
func New(logger *slog.Logger) gormlogger.Interface {
	return NewWithConfig(logger, DefaultConfig())
}

// DefaultConfig returns an independent copy of the SQL logging defaults:
// errors and queries slower than 200 ms, no record-not-found errors or parameters.
func DefaultConfig() gormlogger.Config {
	return gormlogger.Config{
		LogLevel:                  gormlogger.Warn,
		SlowThreshold:             200 * time.Millisecond,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
	}
}

// NewWithConfig uses the complete config, including zero and false values.
// Call it before GORM queries run: it installs a process-wide recorder filter
// that hides Scan parameters, regardless of ParameterizedQueries. That option
// controls ordinary queries. Filtering does not change database parameters.
func NewWithConfig(logger *slog.Logger, config gormlogger.Config) gormlogger.Interface {
	configureRecorder()
	return gormlogger.NewSlogLogger(logger, config)
}
