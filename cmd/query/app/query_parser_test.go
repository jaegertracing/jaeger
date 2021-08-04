// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package app

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var errParseInt = `strconv.ParseInt: parsing "string": invalid syntax`

func TestParseTraceQuery(t *testing.T) {
	timeNow := time.Now()
	const noErr = ""
	tests := []struct {
		urlStr        string
		errMsg        string
		expectedQuery *traceQueryParameters
	}{
		{"", "parameter 'service' is required", nil},
		{"x?service=service&start=string", errParseInt, nil},
		{"x?service=service&end=string", errParseInt, nil},
		{"x?service=service&limit=string", errParseInt, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20", `unable to parse param 'minDuration': time: missing unit in duration "?20"?$`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20s&maxDuration=30", `unable to parse param 'maxDuration': time: missing unit in duration "?30"?$`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tag=x:y&tag=k&log=k:v&log=k", `malformed 'tag' parameter, expecting key:value, received: k`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=25s&maxDuration=1s", `'maxDuration' should be greater than 'minDuration'`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tag=x:y", noErr,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMin:  time.Unix(0, 0),
					StartTimeMax:  time.Unix(0, 0),
					NumTraces:     200,
					Tags:          map[string]string{"k": "v", "x": "y"},
				},
			},
		},
		// tags=JSON with a non-string value 123
		{`x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tags={"x":123}`, "malformed 'tags' parameter, cannot unmarshal JSON: json: cannot unmarshal number into Go value of type string", nil},
		// tags=JSON
		{`x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tags={"x":"y"}`, noErr,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMin:  time.Unix(0, 0),
					StartTimeMax:  time.Unix(0, 0),
					NumTraces:     200,
					Tags:          map[string]string{"k": "v", "x": "y"},
				},
			},
		},
		// tags=url_encode(JSON)
		{`x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tags=%7B%22x%22%3A%22y%22%7D`, noErr,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMin:  time.Unix(0, 0),
					StartTimeMax:  time.Unix(0, 0),
					NumTraces:     200,
					Tags:          map[string]string{"k": "v", "x": "y"},
				},
			},
		},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=10s&maxDuration=20s", noErr,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMin:  time.Unix(0, 0),
					StartTimeMax:  time.Unix(0, 0),
					NumTraces:     200,
					DurationMin:   10 * time.Second,
					DurationMax:   20 * time.Second,
					Tags:          make(map[string]string),
				},
			},
		},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=10s", noErr,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMin:  time.Unix(0, 0),
					StartTimeMax:  time.Unix(0, 0),
					NumTraces:     200,
					DurationMin:   10 * time.Second,
					Tags:          make(map[string]string),
				},
			},
		},
		// trace ID in upper/lower case
		{"x?traceID=1f00&traceID=1E00", noErr,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					NumTraces:    100,
					StartTimeMin: timeNow,
					StartTimeMax: timeNow,
					Tags:         make(map[string]string),
				},
				traceIDs: []model.TraceID{
					model.NewTraceID(0, 0x1f00),
					model.NewTraceID(0, 0x1e00),
				},
			},
		},
		{"x?traceID=100&traceID=x200", `cannot parse traceID param: strconv.ParseUint: parsing "x200": invalid syntax`,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					StartTimeMin: time.Unix(0, 0),
					StartTimeMax: time.Unix(0, 0),
					NumTraces:    100,
					Tags:         make(map[string]string),
				},
				traceIDs: []model.TraceID{
					model.NewTraceID(0, 0x100),
					model.NewTraceID(0, 0x200),
				},
			},
		},
	}
	for _, tc := range tests {
		test := tc // capture loop var
		t.Run(test.urlStr, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, test.urlStr, nil)
			assert.NoError(t, err)
			parser := &queryParser{
				timeNow: func() time.Time {
					return timeNow
				},
			}
			actualQuery, err := parser.parseTraceQueryParams(request)
			if test.errMsg == "" {
				assert.NoError(t, err)
				if !assert.Equal(t, test.expectedQuery, actualQuery) {
					for _, s := range pretty.Diff(test.expectedQuery, actualQuery) {
						t.Log(s)
					}
				}
			} else {
				matched, matcherr := regexp.MatchString(test.errMsg, err.Error())
				require.NoError(t, matcherr)
				assert.True(t, matched, fmt.Sprintf("Error \"%s\" should match \"%s\"", err.Error(), test.errMsg))
			}
		})
	}
}

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
			request, err := http.NewRequest(http.MethodGet, "x?service=foo&groupByOperation="+tc.input, nil)
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
	request, err := http.NewRequest(http.MethodGet, "x?service=foo&step=1000", nil)
	require.NoError(t, err)
	parser := &queryParser{
		timeNow: func() time.Time {
			return time.Now()
		},
	}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, time.Second, *mqp.Step)
}

func TestParseRepeatedServices(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "x?service=foo&service=bar", nil)
	require.NoError(t, err)
	parser := &queryParser{
		timeNow: func() time.Time {
			return time.Now()
		},
	}
	mqp, err := parser.parseMetricsQueryParams(request)
	require.NoError(t, err)
	assert.Equal(t, []string{"foo", "bar"}, mqp.ServiceNames)
}

func TestParseRepeatedSpanKinds(t *testing.T) {
	q := "x?service=foo&spanKind=unspecified&spanKind=internal&spanKind=server&spanKind=client&spanKind=producer&spanKind=consumer"
	request, err := http.NewRequest(http.MethodGet, q, nil)
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
	ts := initializeTestServer()
	defer ts.server.Close()

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
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrorMessage)
		})
	}
}
