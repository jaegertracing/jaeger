// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

func TestParseBool(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"t", true},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"T", true},
		{"1", true},
		{"f", false},
		{"false", false},
		{"FALSE", false},
		{"False", false},
		{"F", false},
		{"0", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "x?service=foo&groupByOperation="+tc.input, http.NoBody)
			require.NoError(t, err)
			timeNow := time.Now()
			parser := &queryParser{
				timeNow: func() time.Time {
					return timeNow
				},
			}
			mqp, err := parser.parseMetricsQueryParams(request)
			require.NoError(t, err)
			assert.Equal(t, tc.want, mqp.GroupByOperation)
		})
	}
}

func TestParseDuration(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "x?service=foo&step=1000", http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{
		timeNow: time.Now,
	}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, time.Second, *mqp.Step)
}

func TestParseTimeLargeValues(t *testing.T) {
	parser := &queryParser{
		timeNow: time.Now,
	}
	for _, tc := range []struct {
		name   string
		value  int64
		toTime func(int64) time.Time
	}{
		{
			// Multiplying by time.Millisecond overflowed this to 1970-01-01T00:00:00.000192Z.
			name:   "milliseconds",
			value:  9223372036854776,
			toTime: time.UnixMilli,
		},
		{
			name:   "microseconds",
			value:  9223372036854776,
			toTime: time.UnixMicro,
		},
		{
			name:   "negative milliseconds",
			value:  -9223372036854776,
			toTime: time.UnixMilli,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("x?ts=%d", tc.value), http.NoBody)
			require.NoError(t, err)
			got, err := parser.parseTime(request, "ts", tc.toTime)
			require.NoError(t, err)
			assert.True(t, tc.toTime(tc.value).Equal(got), "got %v", got)
			assert.NotEqual(t, 1970, got.UTC().Year())
		})
	}
}

func TestParseRepeatedServices(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "x?service=foo&service=bar", http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{
		timeNow: time.Now,
	}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, []string{"foo", "bar"}, mqp.ServiceNames)
}

func TestParseRepeatedSpanKinds(t *testing.T) {
	q := "x?service=foo&spanKind=unspecified&spanKind=internal&spanKind=server&spanKind=client&spanKind=producer&spanKind=consumer"
	request, err := http.NewRequest(http.MethodGet, q, http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{
		timeNow: time.Now,
	}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, []string{
		metrics.SpanKind_SPAN_KIND_UNSPECIFIED.String(),
		metrics.SpanKind_SPAN_KIND_INTERNAL.String(),
		metrics.SpanKind_SPAN_KIND_SERVER.String(),
		metrics.SpanKind_SPAN_KIND_CLIENT.String(),
		metrics.SpanKind_SPAN_KIND_PRODUCER.String(),
		metrics.SpanKind_SPAN_KIND_CONSUMER.String(),
	}, mqp.SpanKinds)
}

func TestParseFiltersSingle(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "x?service=foo&filter=deployment.environment:prod", http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{timeNow: time.Now}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"deployment.environment": "prod"}, mqp.Filters)
}

func TestParseFiltersRepeated(t *testing.T) {
	q := "x?service=foo&filter=deployment.environment:prod&filter=k8s.cluster:us-west-1"
	request, err := http.NewRequest(http.MethodGet, q, http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{timeNow: time.Now}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"deployment.environment": "prod",
		"k8s.cluster":            "us-west-1",
	}, mqp.Filters)
}

func TestParseFiltersWithColonInValue(t *testing.T) {
	// A value containing a colon is preserved verbatim — split only on the first ':'.
	q := "x?service=foo&filter=region:us:west-1"
	request, err := http.NewRequest(http.MethodGet, q, http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{timeNow: time.Now}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"region": "us:west-1"}, mqp.Filters)
}

func TestParseFiltersOmittedYieldsNil(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "x?service=foo", http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{timeNow: time.Now}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Nil(t, mqp.Filters)
}

func TestParseFiltersMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"missing colon", "x?service=foo&filter=keyonly"},
		{"empty key", "x?service=foo&filter=:value"},
		{"empty value", "x?service=foo&filter=key:"},
		{"duplicate key", "x?service=foo&filter=k:v1&filter=k:v2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, tc.raw, http.NoBody)
			require.NoError(t, err)
			parser := &queryParser{timeNow: time.Now}
			_, err = parser.parseMetricsQueryParams(request)
			require.Error(t, err)
			assert.ErrorContains(t, err, "filter")
		})
	}
}

func TestParameterErrors(t *testing.T) {
	ts := initializeTestServer(t)

	for _, tc := range []struct {
		name                       string
		urlPath                    string
		mockedQueryMethod          string
		mockedQueryMethodParamType string
		wantErrorMessage           string
	}{
		{
			name:             "missing services",
			urlPath:          "/api/metrics/calls",
			wantErrorMessage: `unable to parse param 'service': please provide at least one service name`,
		},
		{
			name:             "invalid group by operation",
			urlPath:          "/api/metrics/calls?service=emailservice&groupByOperation=foo",
			wantErrorMessage: `unable to parse param 'groupByOperation': strconv.ParseBool: parsing \"foo\": invalid syntax`,
		},
		{
			name:             "invalid span kinds",
			urlPath:          "/api/metrics/calls?service=emailservice&spanKind=foo",
			wantErrorMessage: `unable to parse param 'spanKind': unsupported span kind: 'foo'`,
		},
		{
			name:             "empty span kind",
			urlPath:          "/api/metrics/calls?service=emailservice&spanKind=",
			wantErrorMessage: `unable to parse param 'spanKind': unsupported span kind: ''`,
		},
		{
			name:             "invalid quantile parameter",
			urlPath:          "/api/metrics/latencies?service=emailservice&quantile=foo",
			wantErrorMessage: `unable to parse param 'quantile': strconv.ParseFloat: parsing \"foo\": invalid syntax`,
		},
		{
			name:             "invalid endTs parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&endTs=foo",
			wantErrorMessage: `unable to parse param 'endTs': strconv.ParseInt: parsing \"foo\": invalid syntax`,
		},
		{
			name:             "invalid lookback parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&lookback=foo",
			wantErrorMessage: `unable to parse param 'lookback': strconv.ParseInt: parsing \"foo\": invalid syntax`,
		},
		{
			name:             "invalid step parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&step=foo",
			wantErrorMessage: `unable to parse param 'step': strconv.ParseInt: parsing \"foo\": invalid syntax`,
		},
		{
			name:             "invalid ratePer parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&ratePer=foo",
			wantErrorMessage: `unable to parse param 'ratePer': strconv.ParseInt: parsing \"foo\": invalid syntax`,
		},
		{
			name:             "zero lookback parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&lookback=0",
			wantErrorMessage: `unable to parse param 'lookback': must be a positive duration`,
		},
		{
			name:             "negative lookback parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&lookback=-1000",
			wantErrorMessage: `unable to parse param 'lookback': must be a positive duration`,
		},
		{
			name:             "zero step parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&step=0",
			wantErrorMessage: `unable to parse param 'step': must be a positive duration`,
		},
		{
			name:             "negative step parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&step=-1000",
			wantErrorMessage: `unable to parse param 'step': must be a positive duration`,
		},
		{
			name:             "zero ratePer parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&ratePer=0",
			wantErrorMessage: `unable to parse param 'ratePer': must be a positive duration`,
		},
		{
			name:             "negative ratePer parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&ratePer=-1000",
			wantErrorMessage: `unable to parse param 'ratePer': must be a positive duration`,
		},
		{
			name:             "overflowing negative lookback parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&lookback=-18446744073709551",
			wantErrorMessage: `unable to parse param 'lookback': value -18446744073709551 is out of range for a duration`,
		},
		{
			name:             "overflowing positive step parameter",
			urlPath:          "/api/metrics/calls?service=emailservice&step=9223372036854776",
			wantErrorMessage: `unable to parse param 'step': value 9223372036854776 is out of range for a duration`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Test
			var response metrics.MetricFamily
			err := getJSON(ts.server.URL+tc.urlPath, &response)

			// Verify
			assert.ErrorContains(t, err, tc.wantErrorMessage)
		})
	}
}
