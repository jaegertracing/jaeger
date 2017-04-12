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

package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	ui "github.com/uber/jaeger/model/json"
)

type testQueryHandler struct{}

func (h *testQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	svc := r.FormValue("service")
	body := []byte("bad json")
	if svc == "svc" {
		response := response{
			Data: []*ui.Trace{
				{TraceID: "traceid"},
			},
		}
		body, _ = json.Marshal(response)
	}
	w.Write(body)
}

func TestGetTraces(t *testing.T) {
	handler := &testQueryHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	query := NewQueryService(server.URL, zap.NewNop())
	traces, err := query.GetTraces("svc", "op", map[string]string{"key": "value"})
	assert.Error(t, err)

	query = NewQueryService(server.URL, zap.NewNop())
	traces, err = query.GetTraces("svc", "op", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.Len(t, traces, 1)
	assert.EqualValues(t, "traceid", traces[0].TraceID)

	traces, err = query.GetTraces("bad_svc", "op", map[string]string{"key": "value"})
	assert.Error(t, err)
}
