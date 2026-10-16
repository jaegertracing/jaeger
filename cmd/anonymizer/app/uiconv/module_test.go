// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestModule_TraceSuccess(t *testing.T) {
	inputFile := "fixtures/trace_success.json"
	outputFile := "fixtures/trace_success_ui_anonymized.json"
	defer os.Remove(outputFile)

	config := Config{
		CapturedFile: inputFile,
		UIFile:       outputFile,
		TraceID:      "00000000000000002be38093ead7a083",
	}
	err := Extract(config, zap.NewNop())
	require.NoError(t, err)

	var trace UITrace
	loadJSON(t, outputFile, &trace)

	for i := range trace.Data {
		for j := range trace.Data[i].Spans {
			assert.Equal(t, model.SpanKindKey, trace.Data[i].Spans[j].Tags[0].Key)
		}
	}
}

func TestModule_TraceIDNormalization(t *testing.T) {
	legacyTraceID := "2be38093ead7a083"
	canonicalTraceID := "00000000000000002be38093ead7a083"

	for _, input := range []string{legacyTraceID, canonicalTraceID} {
		t.Run(input, func(t *testing.T) {
			outputFile := t.TempDir() + "/trace.json"
			err := Extract(Config{CapturedFile: "fixtures/trace_success.json", UIFile: outputFile, TraceID: input}, zap.NewNop())
			require.NoError(t, err)

			var trace UITrace
			loadJSON(t, outputFile, &trace)
			require.Len(t, trace.Data, 1)
			assert.Len(t, trace.Data[0].Spans, 2)
			output, err := os.ReadFile(outputFile)
			require.NoError(t, err)
			assert.Contains(t, string(output), `"traceID":"`+canonicalTraceID+`"`)
		})
	}
}

func TestModule_TraceNonExistent(t *testing.T) {
	inputFile := "fixtures/trace_non_existent.json"
	outputFile := "fixtures/trace_non_existent_ui_anonymized.json"
	defer os.Remove(outputFile)

	config := Config{
		CapturedFile: inputFile,
		UIFile:       outputFile,
		TraceID:      "2be38093ead7a083",
	}
	err := Extract(config, zap.NewNop())
	require.ErrorContains(t, err, "cannot open captured file")
}

func TestModule_TraceOutputFileError(t *testing.T) {
	inputFile := "fixtures/trace_success.json"
	outputFile := "fixtures/trace_success_ui_anonymized.json"
	defer os.Remove(outputFile)

	config := Config{
		CapturedFile: inputFile,
		UIFile:       outputFile,
		TraceID:      "2be38093ead7a083",
	}

	err := os.Chmod("fixtures", 0o550)
	require.NoError(t, err)
	defer os.Chmod("fixtures", 0o755)

	err = Extract(config, zap.NewNop())
	require.ErrorContains(t, err, "cannot create output file")
}
