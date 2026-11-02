// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/uiconv"
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

// TestWriter_TruncatesExistingFile verifies that writer.New() truncates
// existing output files via O_TRUNC, preventing stale data.
func TestWriter_TruncatesExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	capturedFile := filepath.Join(tempDir, "captured.json")
	anonymizedFile := filepath.Join(tempDir, "anonymized.json")
	mappingFile := filepath.Join(tempDir, "mapping.json")

	// Create files with old content that is clearly longer than what writer.New() will write
	oldContent := `{"old":"data","stale":true,"extra":"this_should_be_removed_completely"}`
	err := os.WriteFile(capturedFile, []byte(oldContent), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(anonymizedFile, []byte(oldContent), 0o644)
	require.NoError(t, err)

	// Create writer with existing files - should truncate them
	config := Config{
		MaxSpansCount:  10,
		CapturedFile:   capturedFile,
		AnonymizedFile: anonymizedFile,
		MappingFile:    mappingFile,
	}
	writer, err := New(config, zap.NewNop())
	require.NoError(t, err)
	writer.Close()

	// Verify old content is gone from captured file
	capturedData, err := os.ReadFile(capturedFile)
	require.NoError(t, err)
	require.NotContains(t, string(capturedData), "old")
	require.NotContains(t, string(capturedData), "stale")
	require.NotContains(t, string(capturedData), "extra") // proves no leftover tail
	// Ensure no partial/corrupted JSON remains
	var v any
	require.NoError(t, json.Unmarshal(capturedData, &v))

	// Verify old content is gone from anonymized file
	anonymizedData, err := os.ReadFile(anonymizedFile)
	require.NoError(t, err)
	require.NotContains(t, string(anonymizedData), "old")
	require.NotContains(t, string(anonymizedData), "stale")
	require.NotContains(t, string(anonymizedData), "extra") // proves no leftover tail
	// Ensure no partial/corrupted JSON remains
	require.NoError(t, json.Unmarshal(anonymizedData, &v))
}

func TestWriter_CloseIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	capturedFile := filepath.Join(tempDir, "captured.json")
	anonymizedFile := filepath.Join(tempDir, "anonymized.json")
	mappingFile := filepath.Join(tempDir, "mapping.json")

	config := Config{
		MaxSpansCount:  5,
		CapturedFile:   capturedFile,
		AnonymizedFile: anonymizedFile,
		MappingFile:    mappingFile,
	}

	w, err := New(config, zap.NewNop())
	require.NoError(t, err)

	err = w.WriteSpan(span)
	require.NoError(t, err)

	// Multiple calls to Close() should not error or corrupt files
	w.Close()
	w.Close()
	w.Close()

	var captured []any
	capturedData, err := os.ReadFile(capturedFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(capturedData, &captured))
	require.Len(t, captured, 1)

	var anonymized []any
	anonymizedData, err := os.ReadFile(anonymizedFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(anonymizedData, &anonymized))
	require.Len(t, anonymized, 1)
}

func TestWriter_MaxSpansCountAndUIExtraction(t *testing.T) {
	tempDir := t.TempDir()
	capturedFile := filepath.Join(tempDir, "captured.json")
	anonymizedFile := filepath.Join(tempDir, "anonymized.json")
	mappingFile := filepath.Join(tempDir, "mapping.json")
	uiFile := filepath.Join(tempDir, "ui.json")

	config := Config{
		MaxSpansCount:  2,
		CapturedFile:   capturedFile,
		AnonymizedFile: anonymizedFile,
		MappingFile:    mappingFile,
	}

	w, err := New(config, zap.NewNop())
	require.NoError(t, err)

	spans := []*model.Span{span, span, span}
	for _, s := range spans {
		if err := w.WriteSpan(s); err != nil {
			if errors.Is(err, ErrMaxSpansCountReached) {
				break
			}
		}
	}
	// Calling Close() after loop breaks on ErrMaxSpansCountReached
	w.Close()

	// Subsequent WriteSpan should return ErrMaxSpansCountReached
	err = w.WriteSpan(span)
	require.ErrorIs(t, err, ErrMaxSpansCountReached)

	// Both files must be valid JSON arrays with exactly 2 spans
	var captured []any
	capturedData, err := os.ReadFile(capturedFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(capturedData, &captured))
	require.Len(t, captured, 2)

	var anonymized []any
	anonymizedData, err := os.ReadFile(anonymizedFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(anonymizedData, &anonymized))
	require.Len(t, anonymized, 2)

	// UI extraction must succeed on the capped anonymized file
	err = uiconv.Extract(uiconv.Config{
		CapturedFile: anonymizedFile,
		UIFile:       uiFile,
		TraceID:      traceID.String(),
	}, zap.NewNop())
	require.NoError(t, err)

	uiData, err := os.ReadFile(uiFile)
	require.NoError(t, err)
	var uiObj struct {
		Data []struct {
			Spans []any `json:"spans"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(uiData, &uiObj))
	require.Len(t, uiObj.Data, 1)
	require.Len(t, uiObj.Data[0].Spans, 2)
}
