// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestToDBModel(t *testing.T) {
	trace := jsonToPtrace(t, "./fixtures/ptrace.json")
	input := ToDBModel(trace)

	expected := readJSONBytes(t, "./fixtures/input.json")
	actual, err := json.MarshalIndent(input, "", "  ")
	require.NoError(t, err)
	require.ElementsMatch(t, expected, actual)
}

func jsonToPtrace(t *testing.T, filename string) (trace ptrace.Traces) {
	unMarshaler := ptrace.JSONUnmarshaler{}
	trace, err := unMarshaler.UnmarshalTraces(readJSONBytes(t, filename))
	require.NoError(t, err, "Failed to unmarshal trace with %s", filename)
	return trace
}
