// Package gormlog connects GORM SQL logging to slog.
package gormlog

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// HideScanParameters sets GORM's process-wide recorder filter to omit bound
// values from Scan SQL logs. Call it during application startup before any
// GORM queries run.
func HideScanParameters() {
	gormlogger.RecorderParamsFilter = func(_ context.Context, sql string, _ ...any) (string, []any) {
		return sql, nil
	}
}

// New returns a GORM logger with DefaultConfig.
func New(logger *slog.Logger) gormlogger.Interface {
	return NewWithConfig(logger, DefaultConfig())
}

// DefaultConfig returns settings for errors and queries slower than 200 ms.
// Those settings omit record-not-found errors and ordinary query parameters.
func DefaultConfig() gormlogger.Config {
	return gormlogger.Config{
		LogLevel:                  gormlogger.Warn,
		SlowThreshold:             200 * time.Millisecond,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
	}
}

// NewWithConfig uses the complete config, including zero and false values.
func NewWithConfig(logger *slog.Logger, config gormlogger.Config) gormlogger.Interface {
	// GORM v1.31.2 can emit slow-query warnings in Error mode.
	// Disable the threshold on a copy so LogMode can restore the configured
	// threshold when switching to Warn or Info.
	effective := config
	if effective.LogLevel < gormlogger.Warn {
		effective.SlowThreshold = 0
	}
	return levelLogger{
		Interface: gormlogger.NewSlogLogger(logger, effective),
		slog:      logger,
		config:    config,
	}
}

type levelLogger struct {
	gormlogger.Interface
	slog   *slog.Logger
	config gormlogger.Config
}

func (l levelLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	config := l.config
	config.LogLevel = level
	return NewWithConfig(l.slog, config)
}

// Trace logs queries canceled by the caller at WARN instead of ERROR, for
// example when the HTTP client disconnects. The query did not fail, but how
// long it ran shows what users gave up on. Timeouts and all other SQL errors
// are still logged as errors.
func (l levelLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if !errors.Is(err, context.Canceled) {
		l.Interface.Trace(ctx, begin, fc, err)
		return
	}
	if l.config.LogLevel >= gormlogger.Warn {
		sql, _ := fc()
		l.slog.WarnContext(ctx, "SQL canceled",
			"duration_ms", time.Since(begin).Milliseconds(),
			"sql", sql,
			"error", err.Error())
	}
}

func (l levelLogger) ParamsFilter(ctx context.Context, sql string, params ...any) (string, []any) {
	return l.Interface.(gorm.ParamsFilter).ParamsFilter(ctx, sql, params...)
}
