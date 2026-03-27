// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package uiconv

import (
	"encoding/json"
	"os"
	"strings"
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

	extractor, err := newExtractor(
		outputFile,
		"2be38093ead7a083",
		reader,
		zap.NewNop(),
	)
	require.NoError(t, err)

	// The file is now opened lazily in Run, so the permission error surfaces there.
	err = os.Chmod("fixtures", 0o000)
	require.NoError(t, err)
	defer os.Chmod("fixtures", 0o755)

	err = extractor.Run()
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

	// Pre-populate the output file with content that is unambiguously larger than what the
	// extractor will write (the real output for trace_success.json is ~835 bytes). Use
	// printable 'a' padding so the stale file stays valid JSON and is easy to read in
	// failure output, while still guaranteeing the stale tail corrupts JSON if O_TRUNC is absent.
	staleContent := `{"data": [{"traceID":"stale","spans":[],"processes":{}}], "stale": true, "padding": "` +
		strings.Repeat("a", 4096) + `"}`
	err := os.WriteFile(outputFile, []byte(staleContent), 0o644)
	require.NoError(t, err)

	staleStat, err := os.Stat(outputFile)
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

	finalStat, err := os.Stat(outputFile)
	require.NoError(t, err)

	// Confirm the file shrank, proving O_TRUNC actually exercised the truncation path.
	require.Greater(t, staleStat.Size(), finalStat.Size(),
		"stale file must be larger than extractor output for this test to be meaningful")

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
