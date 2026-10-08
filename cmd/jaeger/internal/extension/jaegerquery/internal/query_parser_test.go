// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
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

func TestDurationUnitsParserRange(t *testing.T) {
	parse := newDurationUnitsParser(time.Millisecond)

	for _, tc := range []struct {
		input string
		want  time.Duration
	}{
		{input: "9223372036854", want: 9223372036854 * time.Millisecond},
		{input: "-9223372036854", want: -9223372036854 * time.Millisecond},
	} {
		t.Run(tc.input, func(t *testing.T) {
			d, err := parse(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, d)
		})
	}

	for _, input := range []string{"9223372036855", "-9223372036855", "288230376151711745", "-9223372036854775807"} {
		t.Run(input, func(t *testing.T) {
			_, err := parse(input)
			require.EqualError(t, err, "duration out of range: '"+input+"'")
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

func TestParseMetricsQueryParamsRejectsInvalidDurations(t *testing.T) {
	ts := initializeTestServer(t)

	for _, param := range []string{lookbackParam, stepParam} {
		for _, tc := range []struct {
			value   string
			wantErr string
		}{
			{value: "0", wantErr: "must be greater than zero"},
			{value: "-60000", wantErr: "must be greater than zero"},
			// These overflow time.Duration when converted from milliseconds.
			{value: "-9223372036854775807", wantErr: "duration out of range: '-9223372036854775807'"},
			{value: "9223372036855", wantErr: "duration out of range: '9223372036855'"},
			{value: "288230376151711745", wantErr: "duration out of range: '288230376151711745'"},
		} {
			t.Run(param+"="+tc.value, func(t *testing.T) {
				var response metrics.MetricFamily
				err := getJSON(ts.server.URL+"/api/metrics/calls?service=emailservice&"+param+"="+tc.value, &response)

				var httpErr *HTTPError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
				assert.Contains(t, httpErr.Body, "unable to parse param '"+param+"': "+tc.wantErr)
			})
		}
	}
}
