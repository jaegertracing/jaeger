// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestFromPtrace(t *testing.T) {
	trace := jsonToTrace(t, "ptrace.json")
	input := FromPtrace(trace)

	expected := readJSONBytes(t, "input.json")
	actual, err := json.MarshalIndent(input, "", "  ")
	require.NoError(t, err)
	compareJSONBytesIgnoringOrder(t, expected, actual)
}

func readJSONBytes(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join(".", filename)
	bytes, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file %s", filename)
	return bytes
}

func jsonToTrace(t *testing.T, filename string) (trace ptrace.Traces) {
	unMarshaler := ptrace.JSONUnmarshaler{}
	trace, err := unMarshaler.UnmarshalTraces(readJSONBytes(t, filename))
	require.NoError(t, err, "Failed to unmarshal trace with %s", filename)
	return trace
}

func compareJSONBytesIgnoringOrder(t *testing.T, expected []byte, actual []byte) {
	t.Helper()
	require.ElementsMatch(t, expected, actual)
}
