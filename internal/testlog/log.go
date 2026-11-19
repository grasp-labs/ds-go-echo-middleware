// Package testlog provides helpers for asserting captured JSON logs in tests.
package testlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Read decodes log records and removes timestamps for stable expectations.
// It preserves output and prints it on failure.
func Read(t testing.TB, output *bytes.Buffer) []map[string]any {
	t.Helper()
	raw := output.String()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("captured logs:\n%s", raw)
		}
	})

	records := []map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			return records
		}
		require.NoError(t, err, "decode log record")
		delete(record, "time")
		records = append(records, record)
	}
}

// Equal compares JSON without depending on object field order or whitespace.
func Equal(t testing.TB, want string, got any) {
	t.Helper()
	actual, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, want, string(actual))
}
