// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// newTraces builds a trace with one span per name, under a single resource and scope.
func newTraces(names ...string) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "serviceName")
	rs.Resource().Attributes().PutBool("foobar", true)
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	for i, name := range names {
		span := spans.AppendEmpty()
		span.SetTraceID([16]byte{1, 2})
		span.SetSpanID([8]byte{byte(i + 1)})
		span.SetName(name)
		span.Attributes().PutBool("error", true)
		span.Attributes().PutStr("http.method", "POST")
		span.Attributes().PutBool("foobar", true)
		span.Events().AppendEmpty().Attributes().PutStr("logKey", "logValue")
	}
	return traces
}

func readTraces(t *testing.T, path string) ptrace.Traces {
	dat, err := os.ReadFile(path)
	require.NoError(t, err)
	var unmarshaler ptrace.JSONUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(dat)
	require.NoError(t, err)
	return traces
}

func newConfig(tempDir string, maxSpansCount int) Config {
	return Config{
		MaxSpansCount:  maxSpansCount,
		CapturedFile:   filepath.Join(tempDir, "captured.json"),
		AnonymizedFile: filepath.Join(tempDir, "anonymized.json"),
		MappingFile:    filepath.Join(tempDir, "mapping.json"),
	}
}

func TestNew(t *testing.T) {
	nopLogger := zap.NewNop()
	tempDir := t.TempDir()

	t.Run("no error", func(t *testing.T) {
		writer, err := New(newConfig(tempDir, 10), nopLogger)
		require.NoError(t, err)
		defer writer.Close()
	})

	t.Run("CapturedFile does not exist", func(t *testing.T) {
		config := newConfig(tempDir, 0)
		config.CapturedFile = tempDir + "/nonexistent_directory/captured.json"
		_, err := New(config, nopLogger)
		require.ErrorContains(t, err, "cannot create output file")
	})

	t.Run("AnonymizedFile does not exist", func(t *testing.T) {
		config := newConfig(tempDir, 0)
		config.AnonymizedFile = tempDir + "/nonexistent_directory/anonymized.json"
		_, err := New(config, nopLogger)
		require.ErrorContains(t, err, "cannot create output file")
	})
}

func TestWriter_WriteTraces(t *testing.T) {
	t.Run("write traces", func(t *testing.T) {
		config := newConfig(t.TempDir(), 10)
		writer, err := New(config, zap.NewNop())
		require.NoError(t, err)

		require.NoError(t, writer.WriteTraces(newTraces("a", "b")))
		require.NoError(t, writer.WriteTraces(newTraces("c")))
		writer.Close()

		captured := readTraces(t, config.CapturedFile)
		require.Equal(t, 3, captured.SpanCount())
		require.Equal(t, "a", captured.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())

		anonymized := readTraces(t, config.AnonymizedFile)
		require.Equal(t, 3, anonymized.SpanCount())
		span := anonymized.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
		require.NotEqual(t, "a", span.Name())
		_, hasCustom := span.Attributes().Get("foobar")
		require.False(t, hasCustom)

		_, err = os.Stat(config.MappingFile)
		require.NoError(t, err)
	})

	t.Run("write traces with MaxSpansCount", func(t *testing.T) {
		writer, err := New(newConfig(t.TempDir(), 1), zap.NewNop())
		require.NoError(t, err)
		defer writer.Close()

		err = writer.WriteTraces(newTraces("a"))
		require.ErrorIs(t, err, ErrMaxSpansCountReached)
	})

	t.Run("write after close", func(t *testing.T) {
		writer, err := New(newConfig(t.TempDir(), 0), zap.NewNop())
		require.NoError(t, err)
		writer.Close()

		err = writer.WriteTraces(newTraces("a"))
		require.EqualError(t, err, "writer is closed")
	})
}

// TestWriter_TruncatesExistingFile verifies that writer.New() truncates
// existing output files via O_TRUNC, preventing stale data.
func TestWriter_TruncatesExistingFile(t *testing.T) {
	config := newConfig(t.TempDir(), 10)

	// Create files with old content that is clearly longer than what the writer will write
	oldContent := `{"old":"data","stale":true,"extra":"this_should_be_removed_completely","padding":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}`
	require.NoError(t, os.WriteFile(config.CapturedFile, []byte(oldContent), 0o644))
	require.NoError(t, os.WriteFile(config.AnonymizedFile, []byte(oldContent), 0o644))

	writer, err := New(config, zap.NewNop())
	require.NoError(t, err)
	writer.Close()

	for _, file := range []string{config.CapturedFile, config.AnonymizedFile} {
		dat, err := os.ReadFile(file)
		require.NoError(t, err)
		require.NotContains(t, string(dat), "old")
		require.NotContains(t, string(dat), "extra") // proves no leftover tail
		// Ensure no partial/corrupted JSON remains
		var v any
		require.NoError(t, json.Unmarshal(dat, &v))
	}
}

func TestWriter_CloseIdempotent(t *testing.T) {
	config := newConfig(t.TempDir(), 5)
	w, err := New(config, zap.NewNop())
	require.NoError(t, err)

	require.NoError(t, w.WriteTraces(newTraces("a")))

	// Multiple calls to Close() should not error or corrupt files
	w.Close()
	w.Close()
	w.Close()

	require.Equal(t, 1, readTraces(t, config.CapturedFile).SpanCount())
	require.Equal(t, 1, readTraces(t, config.AnonymizedFile).SpanCount())
}

func TestWriter_MaxSpansCount(t *testing.T) {
	config := newConfig(t.TempDir(), 2)
	w, err := New(config, zap.NewNop())
	require.NoError(t, err)

	for _, traces := range []ptrace.Traces{newTraces("a"), newTraces("b", "c"), newTraces("d")} {
		if err := w.WriteTraces(traces); err != nil {
			if errors.Is(err, ErrMaxSpansCountReached) {
				break
			}
		}
	}
	// Calling Close() after loop breaks on ErrMaxSpansCountReached
	w.Close()

	// Subsequent WriteTraces should return ErrMaxSpansCountReached
	err = w.WriteTraces(newTraces("e"))
	require.ErrorIs(t, err, ErrMaxSpansCountReached)

	// Both files must be valid OTLP JSON with exactly the first 2 spans
	captured := readTraces(t, config.CapturedFile)
	require.Equal(t, 2, captured.SpanCount())
	require.Equal(t, "b", captured.ResourceSpans().At(1).ScopeSpans().At(0).Spans().At(0).Name())
	require.Equal(t, 2, readTraces(t, config.AnonymizedFile).SpanCount())
}

func TestTruncateDropsEmptyContainers(t *testing.T) {
	traces := newTraces("a", "b")
	newTraces("c").ResourceSpans().MoveAndAppendTo(traces.ResourceSpans())

	truncate(traces, 1)

	require.Equal(t, 1, traces.SpanCount())
	require.Equal(t, 1, traces.ResourceSpans().Len())
}
