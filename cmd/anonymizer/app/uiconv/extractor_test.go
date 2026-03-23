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

func TestExtractorTruncatesOutputOnRerun(t *testing.T) {
	// Regression test for https://github.com/jaegertracing/jaeger/issues/8231:
	// a second run on the same output file must not leave stale bytes from the first run.
	inputFile := "fixtures/trace_success.json"
	outputFile := t.TempDir() + "/out.json"

	runExtractor := func() {
		reader, err := newSpanReader(inputFile, zap.NewNop())
		require.NoError(t, err)
		extractor, err := newExtractor(outputFile, "2be38093ead7a083", reader, zap.NewNop())
		require.NoError(t, err)
		require.NoError(t, extractor.Run())
	}

	// First run — writes normally.
	runExtractor()
	first, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	// Overwrite the file with longer content to simulate a "large previous run".
	require.NoError(t, os.WriteFile(outputFile, append(first, []byte("STALE_GARBAGE")...), 0o600))

	// Second run — must truncate, leaving only fresh output.
	runExtractor()
	second, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	require.Equal(t, first, second, "second run left stale bytes from the first run")

	var trace UITrace
	loadJSON(t, outputFile, &trace)
}

func loadJSON(t *testing.T, fileName string, i any) {
	b, err := os.ReadFile(fileName)
	require.NoError(t, err)
	err = json.Unmarshal(b, i)
	require.NoError(t, err, "Failed to parse json fixture file %s", fileName)
}
