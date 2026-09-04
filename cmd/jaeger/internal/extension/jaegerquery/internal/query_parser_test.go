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

func TestParseTags(t *testing.T) {
	parser := &queryParser{}
	for _, tc := range []struct {
		name       string
		simpleTags []string
		jsonTags   []string
		want       map[string]string
		wantErr    string
	}{
		{
			name:       "simple tag",
			simpleTags: []string{"foo:bar"},
			want:       map[string]string{"foo": "bar"},
		},
		{
			name:       "simple tag with colon in value",
			simpleTags: []string{"foo:bar:baz"},
			want:       map[string]string{"foo": "bar:baz"},
		},
		{
			name:       "multiple simple tags",
			simpleTags: []string{"foo:bar", "key:value"},
			want:       map[string]string{"foo": "bar", "key": "value"},
		},
		{
			name:     "json tag",
			jsonTags: []string{`{"foo":"bar"}`},
			want:     map[string]string{"foo": "bar"},
		},
		{
			name:     "multiple json tag params",
			jsonTags: []string{`{"foo":"bar"}`, `{"key":"value"}`},
			want:     map[string]string{"foo": "bar", "key": "value"},
		},
		{
			name:       "json overrides simple",
			simpleTags: []string{"foo:simple", "key:value"},
			jsonTags:   []string{`{"foo":"json"}`},
			want:       map[string]string{"foo": "json", "key": "value"},
		},
		{
			name: "empty input",
			want: map[string]string{},
		},
		{
			name:       "malformed simple tag",
			simpleTags: []string{"foo"},
			wantErr:    "malformed 'tag' parameter, expecting key:value, received: foo",
		},
		{
			name:     "malformed json tag",
			jsonTags: []string{"foo"},
			wantErr:  "malformed 'tags' parameter, cannot unmarshal JSON",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parser.parseTags(tc.simpleTags, tc.jsonTags)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseMetricsQueryParamsTags(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "x?service=foo&tag=foo:bar&tag=key:value&tags=%7B%22json%22%3A%22tag%22%7D", http.NoBody)
	require.NoError(t, err)
	parser := &queryParser{
		timeNow: time.Now,
	}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"foo": "bar", "key": "value", "json": "tag"}, mqp.Tags)
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
