// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

type UITrace struct {
	Data []model.Trace
}

func TestExtractorTraceSuccess(t *testing.T) {
	inputFile := "fixtures/trace_success.json"
	outputFile := "fixtures/trace_success_ui_anonymized.json"
	defer os.Remove(outputFile)

	reader, err := newSpanReader(inputFile, zap.NewNop())
	require.NoError(t, err)

	extractor, err := newExtractor(
		outputFile,
		"2be38093ead7a083",
		reader,
		zap.NewNop(),
	)
	require.NoError(t, err)

	err = extractor.Run()
	require.NoError(t, err)

	var trace UITrace
	loadJSON(t, outputFile, &trace)

	for i := range trace.Data {
		for j := range trace.Data[i].Spans {
			assert.Equal(t, model.SpanKindKey, trace.Data[i].Spans[j].Tags[0].Key)
		}
	}
}

func TestExtractorTraceOutputFileError(t *testing.T) {
	inputFile := "fixtures/trace_success.json"
	outputFile := "fixtures/trace_success_ui_anonymized.json"
	defer os.Remove(outputFile)

	reader, err := newSpanReader(inputFile, zap.NewNop())
	require.NoError(t, err)

	err = os.Chmod("fixtures", 0o000)
	require.NoError(t, err)
	defer os.Chmod("fixtures", 0o755)

	_, err = newExtractor(
		outputFile,
		"2be38093ead7a083",
		reader,
		zap.NewNop(),
	)
	require.ErrorContains(t, err, "cannot create output file")
}

func TestExtractorTraceScanError(t *testing.T) {
	inputFile := "fixtures/trace_scan_error.json"
	outputFile := "fixtures/trace_scan_error_ui_anonymized.json"
	defer os.Remove(outputFile)

	reader, err := newSpanReader(inputFile, zap.NewNop())
	require.NoError(t, err)

	extractor, err := newExtractor(
		outputFile,
		"2be38093ead7a083",
		reader,
		zap.NewNop(),
	)
	require.NoError(t, err)

	err = extractor.Run()
	require.ErrorContains(t, err, "failed when scanning the file")
}

func TestExtractorOutputFileTruncated(t *testing.T) {
	inputFile := "fixtures/trace_success.json"
	outputFile := "fixtures/trace_truncation_test_ui_anonymized.json"
	defer os.Remove(outputFile)

	// Pre-populate the output file with content that is longer than what the extractor will write,
	// simulating a previous run with a larger result. This verifies that stale data at the end of
	// the file is removed when the new (shorter) output is written.
	staleContent := `{"data": [{"traceID":"stale","spans":[],"processes":{}}], "stale": true}`
	err := os.WriteFile(outputFile, []byte(staleContent), 0o644)
	require.NoError(t, err)

	reader, err := newSpanReader(inputFile, zap.NewNop())
	require.NoError(t, err)

	extractor, err := newExtractor(
		outputFile,
		"2be38093ead7a083",
		reader,
		zap.NewNop(),
	)
	require.NoError(t, err)

	err = extractor.Run()
	require.NoError(t, err)

	// The output must be valid JSON with no stale trailing bytes.
	var result map[string]any
	loadJSON(t, outputFile, &result)

	// Confirm stale key from the previous run is gone.
	_, hasStale := result["stale"]
	assert.False(t, hasStale, "output file should not contain stale data from a previous run")
}

func loadJSON(t *testing.T, fileName string, i any) {
	b, err := os.ReadFile(fileName)
	require.NoError(t, err)
	err = json.Unmarshal(b, i)
	require.NoError(t, err, "Failed to parse json fixture file %s", fileName)
}
