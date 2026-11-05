package gormlog

import (
	"bytes"
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

func TestScanRecorderHidesBoundValues(t *testing.T) {
	New(slog.Default())
	sql, params := gormlogger.Recorder.New().ParamsFilter(t.Context(), "SELECT ?", "private-value")
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
