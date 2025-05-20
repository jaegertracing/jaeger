// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func readJSONBytes(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join(".", filename)
	bytes, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file %s", filename)
	return bytes
}

func jsonToPtrace(t *testing.T, filename string) (trace ptrace.Traces) {
	unMarshaler := ptrace.JSONUnmarshaler{}
	trace, err := unMarshaler.UnmarshalTraces(readJSONBytes(t, filename))
	require.NoError(t, err, "Failed to unmarshal trace with %s", filename)
	return trace
}

func jsonToDBModel(t *testing.T, filename string) (trace Trace) {
	traceBytes := readJSONBytes(t, filename)
	err := json.Unmarshal(traceBytes, &trace)
	require.NoError(t, err, "Failed to read file %s", filename)
	return trace
}
