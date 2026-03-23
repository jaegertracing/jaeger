// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var tags = []model.KeyValue{
	model.Bool("error", true),
	model.String("http.method", http.MethodPost),
	model.Bool("foobar", true),
}

var traceID = model.NewTraceID(1, 2)

var span = &model.Span{
	TraceID: traceID,
	SpanID:  model.NewSpanID(1),
	Process: &model.Process{
		ServiceName: "serviceName",
		Tags:        tags,
	},
	OperationName: "operationName",
	Tags:          tags,
	Logs: []model.Log{
		{
			Timestamp: time.Now(),
			Fields: []model.KeyValue{
				model.String("logKey", "logValue"),
			},
		},
	},
	Duration:  time.Second * 5,
	StartTime: time.Unix(300, 0),
}

func TestNew(t *testing.T) {
	nopLogger := zap.NewNop()
	tempDir := t.TempDir()

	t.Run("no error", func(t *testing.T) {
		config := Config{
			MaxSpansCount:  10,
			CapturedFile:   tempDir + "/captured.json",
			AnonymizedFile: tempDir + "/anonymized.json",
			MappingFile:    tempDir + "/mapping.json",
		}
		writer, err := New(config, nopLogger)
		require.NoError(t, err)
		defer writer.Close()
	})

	t.Run("CapturedFile does not exist", func(t *testing.T) {
		config := Config{
			CapturedFile:   tempDir + "/nonexistent_directory/captured.json",
			AnonymizedFile: tempDir + "/anonymized.json",
			MappingFile:    tempDir + "/mapping.json",
		}
		_, err := New(config, nopLogger)
		require.ErrorContains(t, err, "cannot create output file")
	})

	t.Run("AnonymizedFile does not exist", func(t *testing.T) {
		config := Config{
			CapturedFile:   tempDir + "/captured.json",
			AnonymizedFile: tempDir + "/nonexistent_directory/anonymized.json",
			MappingFile:    tempDir + "/mapping.json",
		}
		_, err := New(config, nopLogger)
		require.ErrorContains(t, err, "cannot create output file")
	})
}

func TestWriterTruncatesOutputOnRerun(t *testing.T) {
	// Regression test for https://github.com/jaegertracing/jaeger/issues/8231:
	// a second Writer run on the same files must not leave stale bytes from the first run.
	nopLogger := zap.NewNop()
	tempDir := t.TempDir()
	config := Config{
		MaxSpansCount:  10,
		CapturedFile:   tempDir + "/captured.json",
		AnonymizedFile: tempDir + "/anonymized.json",
		MappingFile:    tempDir + "/mapping.json",
	}

	// First run — write multiple spans.
	w1, err := New(config, nopLogger)
	require.NoError(t, err)
	for range 5 {
		require.NoError(t, w1.WriteSpan(span))
	}
	w1.Close()

	firstCaptured, err := os.ReadFile(config.CapturedFile)
	require.NoError(t, err)
	firstAnonymized, err := os.ReadFile(config.AnonymizedFile)
	require.NoError(t, err)

	// Second run — write fewer spans; stale bytes must not remain.
	w2, err := New(config, nopLogger)
	require.NoError(t, err)
	require.NoError(t, w2.WriteSpan(span))
	w2.Close()

	secondCaptured, err := os.ReadFile(config.CapturedFile)
	require.NoError(t, err)
	secondAnonymized, err := os.ReadFile(config.AnonymizedFile)
	require.NoError(t, err)

	require.Less(t, len(secondCaptured), len(firstCaptured),
		"second run should produce fewer bytes (fewer spans)")
	require.Less(t, len(secondAnonymized), len(firstAnonymized),
		"second run should produce fewer bytes (fewer spans)")

	// Both outputs must be valid JSON arrays.
	var capArr, anonArr []any
	require.NoError(t, json.Unmarshal(secondCaptured, &capArr), "captured file is not valid JSON")
	require.NoError(t, json.Unmarshal(secondAnonymized, &anonArr), "anonymized file is not valid JSON")
}

func TestWriter_WriteSpan(t *testing.T) {
	nopLogger := zap.NewNop()
	t.Run("write span", func(t *testing.T) {
		tempDir := t.TempDir()
		config := Config{
			MaxSpansCount:  10,
			CapturedFile:   tempDir + "/captured.json",
			AnonymizedFile: tempDir + "/anonymized.json",
			MappingFile:    tempDir + "/mapping.json",
		}

		writer, err := New(config, nopLogger)
		require.NoError(t, err)
		defer writer.Close()

		for range 9 {
			err = writer.WriteSpan(span)
			require.NoError(t, err)
		}
	})
	t.Run("write span with MaxSpansCount", func(t *testing.T) {
		tempDir := t.TempDir()
		config := Config{
			MaxSpansCount:  1,
			CapturedFile:   tempDir + "/captured.json",
			AnonymizedFile: tempDir + "/anonymized.json",
			MappingFile:    tempDir + "/mapping.json",
		}

		writer, err := New(config, zap.NewNop())
		require.NoError(t, err)
		defer writer.Close()

		err = writer.WriteSpan(span)
		require.ErrorIs(t, err, ErrMaxSpansCountReached)
	})
}
