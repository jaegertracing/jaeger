// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

var tags = []model.KeyValue{
	model.Bool("error", true),
	model.String("http.method", "POST"),
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

		for i := 0; i < 9; i++ {
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
