// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	ui "github.com/jaegertracing/jaeger/model/json"
)

type testQueryHandler struct{}

func (*testQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	require.Error(t, err)

	query = NewQueryService(server.URL, zap.NewNop())
	traces, err := query.GetTraces("svc", "op", map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.Len(t, traces, 1)
	assert.EqualValues(t, "traceid", traces[0].TraceID)

	_, err = query.GetTraces("bad_svc", "op", map[string]string{"key": "value"})
	require.Error(t, err)
}

func TestGetTracesReadAllErr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1")
	}))
	defer server.Close()
	query := NewQueryService(server.URL, zap.NewNop())
	_, err := query.GetTraces("svc", "op", map[string]string{"key": "value"})
	require.EqualError(t, err, "unexpected EOF")
}
