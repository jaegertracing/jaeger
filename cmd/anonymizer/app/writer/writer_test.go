// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/crossdock/crossdock-go/assert"
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

	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "successfully create writer",
			config: Config{
				MaxSpansCount:  10,
				CapturedFile:   tempDir + "/captured.json",
				AnonymizedFile: tempDir + "/anonymized.json",
				MappingFile:    tempDir + "/mapping.json",
			},
		},
		{
			name: "captured.json doesn't exist",
			config: Config{
				CapturedFile:   tempDir + "/nonexistent_directory/captured.json",
				AnonymizedFile: tempDir + "/anonymized.json",
				MappingFile:    tempDir + "/mapping.json",
			},
		},
		{
			name: "anonymized.json doesn't exist",
			config: Config{
				CapturedFile:   tempDir + "/captured.json",
				AnonymizedFile: tempDir + "/nonexistent_directory/anonymized.json",
				MappingFile:    tempDir + "/mapping.json",
			},
		},
	}

	t.Run(tests[0].name, func(t *testing.T) {
		_, err := New(tests[0].config, nopLogger)
		require.NoError(t, err)
	})
	t.Run(tests[1].name, func(t *testing.T) {
		_, err := New(tests[1].config, nopLogger)
		require.Error(t, err)
	})
	t.Run(tests[2].name, func(t *testing.T) {
		_, err := New(tests[2].config, nopLogger)
		require.Error(t, err)
	})
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
		assert.NoError(t, err)
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

		err = writer.WriteSpan(span)
		if err == nil {
			t.Fatal("expected an error but got none")
		}

		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestWriter_WriteSpan_Exits")
	cmd.Env = append(os.Environ(), "BE_SUBPROCESS=1")
	err := cmd.Run()
	// The process should have exited with status 1, but exec.Command returns
	// an *ExitError when the process exits with a non-zero status.
	if err != nil {
		t.Fatalf("process ran with err %v, want exit status 1", err)
	}
}
