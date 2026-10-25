// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// otlpOneSpan returns an OTLP/JSON payload with a single span.
func otlpOneSpan(serviceName, spanName string, startNano, endNano int) []byte {
	return []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": %q}}]},
			"scopeSpans": [{"spans": [{
				"traceId": "0102030405060708090a0b0c0d0e0f10",
				"spanId": "0102030405060708",
				"name": %q,
				"startTimeUnixNano": "%d",
				"endTimeUnixNano": "%d"
			}]}]
		}]
	}`, serviceName, spanName, startNano, endNano))
}

// An uploaded trace must be cleaned up the same way as an ingested one.
func TestOTLP2TracesSanitizes(t *testing.T) {
	tests := []struct {
		name          string
		payload       []byte
		wantService   string
		wantOperation string
		wantWarning   string
	}{
		{
			name:          "end before start",
			payload:       otlpOneSpan("svc", "op", 5000, 1000),
			wantService:   "svc",
			wantOperation: "op",
			wantWarning:   "Negative duration detected",
		},
		{
			name:          "empty service name",
			payload:       otlpOneSpan("", "op", 1000, 2000),
			wantService:   "empty-service-name",
			wantOperation: "op",
		},
		{
			name:          "empty span name",
			payload:       otlpOneSpan("svc", "", 1000, 2000),
			wantService:   "svc",
			wantOperation: "empty-span-name",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces, err := otlp2traces(test.payload)
			require.NoError(t, err)
			require.Len(t, traces, 1)
			require.Len(t, traces[0].Spans, 1)

			span := traces[0].Spans[0]
			assert.Equal(t, test.wantService, span.Process.ServiceName)
			assert.Equal(t, test.wantOperation, span.OperationName)
			assert.GreaterOrEqual(t, int64(span.Duration), int64(0), "duration must not be negative")
			if test.wantWarning != "" {
				require.NotEmpty(t, span.Warnings)
				assert.Contains(t, span.Warnings[0], test.wantWarning)
			}
		})
	}
}
