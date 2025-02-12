// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package analytics

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
)

// Utility functions used from http_gateway_test.go.
type testGateway struct {
	url    string
	router *mux.Router
	// used to set a tenancy header when executing requests
	setupRequest func(*http.Request)
}

func (gw *testGateway) execRequest(t *testing.T, url string) ([]byte, int) {
	req, err := http.NewRequest(http.MethodGet, gw.url+url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	gw.setupRequest(req)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return body, response.StatusCode
}

func runGatewayTests(
	t *testing.T,
	basePath string,
	setupRequest func(*http.Request),
) {
	gw := setupHTTPGateway(t, basePath)
	gw.setupRequest = setupRequest
	t.Run("getDeepDependencies", gw.runGatewayGetDeepDependencies)
}

func (gw *testGateway) runGatewayGetDeepDependencies(t *testing.T) {
	expectedPath := []TDdgPayloadEntry{
		{Service: "sample-serviceA", Operation: "sample-opA"},
		{Service: "sample-serviceB", Operation: "sample-opB"},
	}
	expectedAttributes := []Attribute{
		{Key: "exemplar_trace_id", Value: "abc123"},
	}

	body, statusCode := gw.execRequest(t, "/api/deep-dependencies")

	require.Equal(t, http.StatusOK, statusCode)

	var response TDdgPayload
	err := json.Unmarshal(body, &response)
	require.NoError(t, err)

	require.Len(t, response.Dependencies, 1)
	require.Equal(t, expectedPath, response.Dependencies[0].Path)
	require.Equal(t, expectedAttributes, response.Dependencies[0].Attributes)
}
