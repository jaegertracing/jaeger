// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package uiconv

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
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
			assert.Equal(t, "span.kind", trace.Data[i].Spans[j].Tags[0].Key)
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
	require.Contains(t, err.Error(), "cannot create output file")
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
	require.Contains(t, err.Error(), "failed when scanning the file")
}

func loadJSON(t *testing.T, fileName string, i any) {
	b, err := os.ReadFile(fileName)
	require.NoError(t, err)
	err = json.Unmarshal(b, i)
	require.NoError(t, err, "Failed to parse json fixture file %s", fileName)
}
