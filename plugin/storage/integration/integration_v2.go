// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"testing"

	"github.com/kr/pretty"
	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type StorageIntegrationV2 struct {
	TraceReader tracestore.Reader
	TraceWriter tracestore.Writer
}

// This function is used to compare traces in new format := v2 function
func (s *StorageIntegrationV2) CompareTraces(t *testing.T, expected ptrace.Traces, actual ptrace.Traces) {
	// Convert both traces to Jaeger format for comparison
	expectedBatches := otlp2jaeger.ProtoFromTraces(expected)
	actualBatches := otlp2jaeger.ProtoFromTraces(actual)

	// First check the number of batches
	require.Equal(t, len(expectedBatches), len(actualBatches), "Unequal number of expected vs. actual batches")

	// Convert batches to traces for comparison
	var expectedTrace, actualTrace model.Trace
	for _, batch := range expectedBatches {
		expectedTrace.Spans = append(expectedTrace.Spans, batch.Spans...)
	}
	for _, batch := range actualBatches {
		actualTrace.Spans = append(actualTrace.Spans, batch.Spans...)
	}

	// Remove duplicate spans if any
	dedupeSpans(&actualTrace)

	// Sort both traces
	model.SortTrace(&expectedTrace)
	model.SortTrace(&actualTrace)

	// Check sizes match
	checkSize(t, &expectedTrace, &actualTrace)

	// Compare using pretty.Diff
	if diff := pretty.Diff(&expectedTrace, &actualTrace); len(diff) > 0 {
		for _, d := range diff {
			t.Logf("Expected and actual differ: %v\n", d)
		}

		// Marshal actual trace for debugging
		out, err := json.Marshal(&actualTrace)
		require.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))

		// Marshal expected trace for debugging
		expected, err := json.Marshal(&expectedTrace)
		require.NoError(t, err)
		t.Logf("Expected trace: %s", string(expected))

		t.Fail()
	}
}


