// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleScrape = `
# HELP otelcol_receiver_accepted_spans Number of spans accepted
# TYPE otelcol_receiver_accepted_spans counter
otelcol_receiver_accepted_spans{receiver="otlp",transport="grpc"} 42
# HELP otelcol_exporter_sent_spans Number of spans sent
# TYPE otelcol_exporter_sent_spans counter
otelcol_exporter_sent_spans{exporter="jaeger_storage_exporter"} 42
# HELP jaeger_storage_requests Storage requests
# TYPE jaeger_storage_requests counter
jaeger_storage_requests{operation="get_trace",result="ok"} 7
# HELP jaeger_storage_latency Storage latency
# TYPE jaeger_storage_latency histogram
jaeger_storage_latency_bucket{operation="get_trace",le="+Inf"} 7
jaeger_storage_latency_sum{operation="get_trace"} 0.01
jaeger_storage_latency_count{operation="get_trace"} 7
# HELP target_info Target metadata
# TYPE target_info gauge
target_info{service_name="jaeger"} 1
`

func TestParseMetricExposition_FamiliesAndValues(t *testing.T) {
	exp := parseMetricExposition(sampleScrape)

	assert.Equal(t, "counter", exp.families["otelcol_receiver_accepted_spans"])
	assert.Equal(t, "counter", exp.families["jaeger_storage_requests"])
	assert.Equal(t, "histogram", exp.families["jaeger_storage_latency"])
	assert.NotContains(t, exp.families, "not_a_metric")

	v, ok := exp.sampleValueForFamily("otelcol_receiver_accepted_spans")
	require.True(t, ok)
	assert.Equal(t, 42.0, v)

	v, ok = exp.sampleValueForFamily("jaeger_storage_latency")
	require.True(t, ok)
	assert.Equal(t, 7.0, v) // max of bucket/sum/count samples
}

func TestParseSampleLine(t *testing.T) {
	tests := []struct {
		line      string
		wantName  string
		wantValue float64
		wantOK    bool
	}{
		{`otelcol_receiver_accepted_spans{receiver="otlp"} 12`, "otelcol_receiver_accepted_spans", 12, true},
		{`jaeger_storage_requests 3`, "jaeger_storage_requests", 3, true},
		{`jaeger_storage_requests 3 1395066363000`, "jaeger_storage_requests", 3, true},
		{`# TYPE foo counter`, "", 0, false},
		{``, "", 0, false},
	}
	for _, tc := range tests {
		name, value, ok := parseSampleLine(tc.line)
		assert.Equal(t, tc.wantOK, ok, tc.line)
		if tc.wantOK {
			assert.Equal(t, tc.wantName, name, tc.line)
			assert.Equal(t, tc.wantValue, value, tc.line)
		}
	}
}

func TestCheckCoreMetrics_OK(t *testing.T) {
	require.NoError(t, checkCoreMetrics(sampleScrape))
}

func TestCheckCoreMetrics_MissingFamily(t *testing.T) {
	body := strings.ReplaceAll(sampleScrape, "otelcol_exporter_sent_spans", "other_exporter_spans")
	err := checkCoreMetrics(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "otelcol_exporter_sent_spans")
	assert.Contains(t, err.Error(), "core metrics missing")
}

func TestCheckCoreMetrics_ZeroCounter(t *testing.T) {
	body := `
# TYPE otelcol_receiver_accepted_spans counter
otelcol_receiver_accepted_spans{receiver="otlp"} 0
# TYPE otelcol_exporter_sent_spans counter
otelcol_exporter_sent_spans{exporter="jaeger_storage_exporter"} 1
# TYPE jaeger_storage_requests counter
jaeger_storage_requests{operation="get_trace"} 0
# TYPE jaeger_storage_latency histogram
jaeger_storage_latency_count{operation="get_trace"} 0
`
	err := checkCoreMetrics(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "never incremented")
	assert.Contains(t, err.Error(), "otelcol_receiver_accepted_spans")
}

func TestAssertCoreMetricsPresent_OK(t *testing.T) {
	assertCoreMetricsPresent(t, sampleScrape)
}
