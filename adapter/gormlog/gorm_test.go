package gormlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/testlog"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/logctx"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestScanRecorderRedactionRequiresExplicitSetup(t *testing.T) {
	previousFilter := gormlogger.RecorderParamsFilter
	t.Cleanup(func() { gormlogger.RecorderParamsFilter = previousFilter })
	gormlogger.RecorderParamsFilter = func(_ context.Context, sql string, params ...any) (string, []any) {
		return sql, params
	}

	New(slog.Default())
	recorder := gormlogger.Recorder.New()
	sql, params := recorder.ParamsFilter(t.Context(), "SELECT ?", "private-value")
	require.Equal(t, "SELECT ?", sql)
	require.Equal(t, []any{"private-value"}, params)

	HideScanParameters()
	sql, params = recorder.ParamsFilter(t.Context(), "SELECT ?", "private-value")
	require.Equal(t, "SELECT ?", sql)
	require.Empty(t, params)
}

func TestGORMTrace(t *testing.T) {
	for _, tt := range []struct {
		name    string
		elapsed time.Duration
		err     error
		level   string
	}{
		{name: "fast success"},
		{name: "slow success", elapsed: 250 * time.Millisecond, level: "WARN"},
		{name: "query error", err: errors.New("database unavailable"), level: "ERROR"},
		{name: "record not found", err: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound)},
		{name: "canceled by caller", err: fmt.Errorf("query: %w", context.Canceled), level: "WARN"},
		{name: "slow and canceled logs once", elapsed: 250 * time.Millisecond, err: context.Canceled, level: "WARN"},
		{name: "deadline exceeded", err: fmt.Errorf("query: %w", context.DeadlineExceeded), level: "ERROR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var output bytes.Buffer
				logger := New(logctx.New(&output))
				ctx := requestctx.SetRequestID(t.Context(), "request-1")
				logger.Trace(ctx, time.Now().Add(-tt.elapsed), func() (string, int64) {
					return "SELECT 1", 1
				}, tt.err)
				records := testlog.Read(t, &output)
				if tt.level == "" {
					require.Empty(t, records)
					return
				}
				require.Len(t, records, 1)
				require.Equal(t, tt.level, records[0]["level"])
				require.Equal(t, "request-1", records[0]["request_id"])
			})
		})
	}
}

func TestConfiguredGORMTrace(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(*gormlogger.Config)
		elapsed   time.Duration
		err       error
		level     string
	}{
		{
			name: "zero threshold disables slow query logging",
			configure: func(config *gormlogger.Config) {
				config.SlowThreshold = 0
			},
			elapsed: time.Second,
		},
		{
			name: "error level logs query errors",
			configure: func(config *gormlogger.Config) {
				config.LogLevel = gormlogger.Error
			},
			elapsed: time.Second, err: errors.New("database unavailable"), level: "ERROR",
		},
		{
			name: "error level never logs a canceled query as an error",
			configure: func(config *gormlogger.Config) {
				config.LogLevel = gormlogger.Error
			},
			err: context.Canceled,
		},
		{
			name: "record not found can be logged",
			configure: func(config *gormlogger.Config) {
				config.IgnoreRecordNotFoundError = false
			},
			err: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), level: "ERROR",
		},
		{
			name: "info logs fast queries",
			configure: func(config *gormlogger.Config) {
				config.LogLevel = gormlogger.Info
			},
			level: "INFO",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				config := DefaultConfig()
				tt.configure(&config)
				var output bytes.Buffer
				logger := NewWithConfig(logctx.New(&output), config)
				ctx := requestctx.SetRequestID(t.Context(), "request-1")
				logger.Trace(ctx, time.Now().Add(-tt.elapsed), func() (string, int64) {
					return "SELECT * FROM state WHERE id = $1", 1
				}, tt.err)
				records := testlog.Read(t, &output)
				if tt.level == "" {
					require.Empty(t, records)
					return
				}
				require.Len(t, records, 1)
				require.Equal(t, tt.level, records[0]["level"])
				require.Equal(t, "request-1", records[0]["request_id"])
			})
		})
	}
}

func TestCustomConfigLeavesDefaultsUnchanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := DefaultConfig()
		config.SlowThreshold = 500 * time.Millisecond
		var output bytes.Buffer
		custom := NewWithConfig(logctx.New(&output), config)
		defaults := New(logctx.New(&output))
		for _, logger := range []gormlogger.Interface{custom, defaults} {
			logger.Trace(t.Context(), time.Now().Add(-250*time.Millisecond), func() (string, int64) {
				return "SELECT 1", 1
			}, nil)
		}
		records := testlog.Read(t, &output)
		require.Len(t, records, 1)
		require.Equal(t, "WARN", records[0]["level"])
	})
}

func TestErrorModeSuppressesSlowQueryWarnings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var output bytes.Buffer
		log := logctx.New(&output)
		config := DefaultConfig()
		config.LogLevel = gormlogger.Error
		for _, logger := range []gormlogger.Interface{
			NewWithConfig(log, config),
			New(log).LogMode(gormlogger.Error),
		} {
			slowStart := time.Now().Add(-time.Second)
			query := func() (string, int64) { return "SELECT 1", 1 }
			logger.Trace(t.Context(), slowStart, query, nil)
			require.Empty(t, output.String())

			logger.LogMode(gormlogger.Warn).Trace(t.Context(), slowStart, query, nil)
			records := testlog.Read(t, &output)
			require.Len(t, records, 1)
			require.Equal(t, "WARN", records[0]["level"])
			output.Reset()
		}
	})
}

func TestLogModePreservesParamsFilter(t *testing.T) {
	for _, parameterized := range []bool{true, false} {
		t.Run(fmt.Sprintf("parameterized=%t", parameterized), func(t *testing.T) {
			config := DefaultConfig()
			config.ParameterizedQueries = parameterized
			var output bytes.Buffer
			logger := NewWithConfig(logctx.New(&output), config).LogMode(gormlogger.Info)
			filter, ok := logger.(gorm.ParamsFilter)
			require.True(t, ok)
			sql, params := filter.ParamsFilter(t.Context(), "SELECT ?", "private-value")
			require.Equal(t, "SELECT ?", sql)
			if parameterized {
				require.Empty(t, params)
			} else {
				require.Equal(t, []any{"private-value"}, params)
			}
		})
	}
}

func TestCanceledQueryKeepsDurationAndError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var output bytes.Buffer
		logger := New(logctx.New(&output))
		logger.Trace(t.Context(), time.Now().Add(-1500*time.Millisecond), func() (string, int64) {
			return "SELECT count(*) FROM xml", -1
		}, fmt.Errorf("failed to connect: %w", context.Canceled))

		records := testlog.Read(t, &output)
		require.Len(t, records, 1)
		require.Equal(t, "SQL canceled", records[0]["msg"])
		require.Equal(t, "SELECT count(*) FROM xml", records[0]["sql"])
		require.EqualValues(t, 1500, records[0]["duration_ms"])
		require.Equal(t, "failed to connect: context canceled", records[0]["error"])
	})
}
