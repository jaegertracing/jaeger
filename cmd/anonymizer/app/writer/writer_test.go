// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"os"
	"os/exec"
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

	config := Config{
		MaxSpansCount:  10,
		CapturedFile:   tempDir + "/captured.json",
		AnonymizedFile: tempDir + "/anonymized.json",
		MappingFile:    tempDir + "/mapping.json",
	}
	_, err := New(config, nopLogger)
	require.NoError(t, err)

	config = Config{
		CapturedFile:   tempDir + "/nonexistent_directory/captured.json",
		AnonymizedFile: tempDir + "/anonymized.json",
		MappingFile:    tempDir + "/mapping.json",
	}
	_, err = New(config, nopLogger)
	require.Error(t, err)

	config = Config{
		CapturedFile:   tempDir + "/captured.json",
		AnonymizedFile: tempDir + "/nonexistent_directory/anonymized.json",
		MappingFile:    tempDir + "/mapping.json",
	}
	_, err = New(config, nopLogger)
	require.Error(t, err)
}

func TestWriter_WriteSpan(t *testing.T) {
	nopLogger := zap.NewNop()
	tempDir := t.TempDir()

	config := Config{
		MaxSpansCount:  101,
		CapturedFile:   tempDir + "/captured.json",
		AnonymizedFile: tempDir + "/anonymized.json",
		MappingFile:    tempDir + "/mapping.json",
	}

	writer, err := New(config, nopLogger)
	require.NoError(t, err)

	for i := 0; i <= 99; i++ {
		err = writer.WriteSpan(span)
		require.NoError(t, err)
	}
}

func TestWriter_WriteSpan_Exits(t *testing.T) {
	if os.Getenv("BE_SUBPROCESS") == "1" {
		tempDir := t.TempDir()
		config := Config{
			MaxSpansCount:  1,
			CapturedFile:   tempDir + "/captured.json",
			AnonymizedFile: tempDir + "/anonymized.json",
			MappingFile:    tempDir + "/mapping.json",
		}

		writer, err := New(config, zap.NewNop())
		require.NoError(t, err)

		err = writer.WriteSpan(span)
		require.NoError(t, err)

		require.Error(t, writer.WriteSpan(span))

		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestWriter_WriteSpan_Exits")
	cmd.Env = append(os.Environ(), "BE_SUBPROCESS=1")
	err := cmd.Run()
	// The process should have exited with status 1, but exec.Command returns
	// an *ExitError when the process exits with a non-zero status.
	require.NoError(t, err)
}
