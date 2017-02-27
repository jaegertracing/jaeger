// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/storage/spanstore"
)

var errParseInt = `strconv.ParseInt: parsing "string": invalid syntax`

func TestParseTraceQuery(t *testing.T) {
	tests := []struct {
		urlStr        string
		errMsg        string
		expectedQuery *spanstore.TraceQueryParameters
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
			&spanstore.TraceQueryParameters{
				ServiceName:   "service",
				OperationName: "operation",
				StartTimeMin:  time.Unix(0, 0),
				StartTimeMax:  time.Unix(0, 0),
				NumTraces:     200,
				Tags:          map[string]string{"k": "v", "x": "y"},
			},
		},
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=10s&maxDuration=20s", ``,
			&spanstore.TraceQueryParameters{
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
		{"x?service=service&start=0&end=0&operation=operation&limit=200&minDuration=10s", ``,
			&spanstore.TraceQueryParameters{
				ServiceName:   "service",
				OperationName: "operation",
				StartTimeMin:  time.Unix(0, 0),
				StartTimeMax:  time.Unix(0, 0),
				NumTraces:     200,
				DurationMin:   10 * time.Second,
				Tags:          make(map[string]string),
			},
		},
	}
	for _, test := range tests {
		request, err := http.NewRequest(http.MethodGet, test.urlStr, nil)
		assert.NoError(t, err)
		parser := &queryParser{}
		actualQuery, err := parser.parse(request)
		if test.errMsg == "" {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, test.errMsg)
		}
		if !assert.Equal(t, test.expectedQuery, actualQuery) {
			for _, s := range pretty.Diff(test.expectedQuery, actualQuery) {
				t.Log(s)
			}
		}
	}
}
