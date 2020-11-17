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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestModule_TraceSuccess(t *testing.T) {
	inputFile := "fixtures/trace_success.json"
	outputFile := "fixtures/trace_success_ui_anonymized.json"
	defer os.Remove(outputFile)

	config := Config{
		CapturedFile: inputFile,
		UIFile:       outputFile,
		TraceID:      "2be38093ead7a083",
	}
	err := Extract(config, zap.NewNop())
	require.NoError(t, err)

	var trace UITrace
	loadJSON(t, outputFile, &trace)

	for i := range trace.Data {
		for j := range trace.Data[i].Spans {
			assert.Equal(t, "span.kind", trace.Data[i].Spans[j].Tags[0].Key)
		}
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
	require.Contains(t, err.Error(), "cannot open captured file")
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

	err := os.Chmod("fixtures", 0550)
	require.NoError(t, err)
	defer os.Chmod("fixtures", 0755)

	err = Extract(config, zap.NewNop())
	require.Contains(t, err.Error(), "cannot create output file")
}
