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

package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	ui "github.com/jaegertracing/jaeger/model/json"
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

	// Test with no http server
	query := NewQueryService("", zap.NewNop())
	_, err := query.GetTraces("svc", "op", map[string]string{"key": "value"})
	assert.Error(t, err)

	query = NewQueryService(server.URL, zap.NewNop())
	traces, err := query.GetTraces("svc", "op", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.Len(t, traces, 1)
	assert.EqualValues(t, "traceid", traces[0].TraceID)

	_, err = query.GetTraces("bad_svc", "op", map[string]string{"key": "value"})
	assert.Error(t, err)
}
