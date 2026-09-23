// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestWriteTracesRoundTrips(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op")
	span.SetTraceID([16]byte{1})
	span.SetSpanID([8]byte{2})

	path := filepath.Join(t.TempDir(), "trace.json")
	require.NoError(t, WriteTraces(path, traces))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(permUserRW), info.Mode().Perm())

	dat, err := os.ReadFile(path)
	require.NoError(t, err)
	var unmarshaler ptrace.JSONUnmarshaler
	got, err := unmarshaler.UnmarshalTraces(dat)
	require.NoError(t, err)
	assert.Equal(t, traces, got)
}

func TestWriteTracesReportsAnUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "trace.json")
	err := WriteTraces(path, ptrace.NewTraces())
	require.ErrorContains(t, err, "cannot write output file")
}
