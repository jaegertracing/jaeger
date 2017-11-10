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
	"net/http"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var errParseInt = `strconv.ParseInt: parsing "string": invalid syntax`

func TestParseTraceQuery(t *testing.T) {
	timeNow := time.Now()
	tests := []struct {
		urlStr        string
		errMsg        string
		expectedQuery *traceQueryParameters
	}{
		{"", "Parameter 'service' is required", nil},
		{"x?service=service&start=string", errParseInt, nil},
		{"x?service=service&end=string", errParseInt, nil},
		{"x?service=service&limit=string", errParseInt, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20", "Could not parse minDuration: time: missing unit in duration 20", nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20s&maxDuration=30", "Could not parse maxDuration: time: missing unit in duration 30", nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&minDuration=1s", `Cannot query for tags when 'minDuration' is specified`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tag=x:y&tag=k&log=k:v&log=k", `Malformed 'tag' parameter, expecting key:value, received: k`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=25s&maxDuration=1s", `'maxDuration' should be greater than 'minDuration'`, nil},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&tag=k:v&tag=x:y", ``,
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
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=10s&maxDuration=20s", ``,
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
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=10s", ``,
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
		{"x?traceID=100&traceID=200", ``,
			&traceQueryParameters{
				TraceQueryParameters: spanstore.TraceQueryParameters{
					NumTraces:    100,
					StartTimeMin: timeNow,
					StartTimeMax: timeNow,
					Tags:         make(map[string]string),
				},
				traceIDs: []model.TraceID{
					{Low: 0x100},
					{Low: 0x200},
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
					{Low: 0x100},
					{Low: 0x200},
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
			actualQuery, err := parser.parse(request)
			if test.errMsg == "" {
				assert.NoError(t, err)
				if !assert.Equal(t, test.expectedQuery, actualQuery) {
					for _, s := range pretty.Diff(test.expectedQuery, actualQuery) {
						t.Log(s)
					}
				}
			} else {
				assert.EqualError(t, err, test.errMsg)
			}
		})
	}
}
