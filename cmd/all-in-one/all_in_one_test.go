// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/ports"
)

// These tests are only run when the environment variable TEST_MODE=integration is set.

const (
	host = "0.0.0.0"

	getServicesURL         = "/api/services"
	getTraceURL            = "/api/traces/"
	getServicesAPIV3URL    = "/api/v3/services"
	getSamplingStrategyURL = "/api/sampling?service=whatever"
)

var (
	queryAddr    = fmt.Sprintf("http://%s:%d", host, ports.QueryHTTP)
	samplingAddr = fmt.Sprintf("http://%s:%d", host, ports.CollectorHTTP)
	healthAddr   = fmt.Sprintf("http://%s:%d/status", host, ports.CollectorV2HealthChecks)
)

var traceID string // stores state exchanged between createTrace and getAPITrace

var httpClient = &http.Client{
	Timeout: time.Second,
}

func TestAllInOne(t *testing.T) {
	if os.Getenv("TEST_MODE") != "integration" {
		t.Skip("Integration test for all-in-one skipped; set environment variable TEST_MODE=integration to enable")
	}

	// Check if the query service is available
	healthCheck(t)

	t.Run("healthCheckV2", healthCheckV2)
	t.Run("checkWebUI", checkWebUI)
	t.Run("createTrace", createTrace)
	t.Run("getAPITrace", getAPITrace)
	t.Run("getSamplingStrategy", getSamplingStrategy)
	t.Run("getServicesAPIV3", getServicesAPIV3)
}

func healthCheck(t *testing.T) {
	require.Eventuallyf(t,
		func() bool {
			resp, err := http.Get(queryAddr + "/")
			if err == nil {
				resp.Body.Close()
			}
			return err == nil
		},
		10*time.Second,
		time.Second,
		"expecting query endpoint to be healthy",
	)
	t.Logf("Server detected at %s", queryAddr)
}

func healthCheckV2(t *testing.T) {
	if os.Getenv("HEALTHCHECK_V2") == "false" {
		t.Skip("Skipping health check for V1 Binary")
	}
	require.Eventuallyf(t,
		func() bool {
			resp, err := http.Get(healthAddr)
			if err == nil {
				resp.Body.Close()
			}
			return err == nil
		},
		10*time.Second,
		time.Second,
		"expecting health endpoint to be healthy",
	)
	t.Logf("V2-HealthCheck Server detected at %s", healthAddr)
}

func httpGet(t *testing.T, url string) (*http.Response, []byte) {
	t.Logf("Executing HTTP GET %s", url)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Close = true // avoid persistent connections which leak goroutines

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, bodyBytes
}

func checkWebUI(t *testing.T) {
	_, bodyBytes := httpGet(t, queryAddr+"/")
	body := string(bodyBytes)
	t.Run("Static_files", func(t *testing.T) {
		pattern := regexp.MustCompile(`<link rel="shortcut icon"[^<]+href="(.*?)"`)
		match := pattern.FindStringSubmatch(body)
		require.Len(t, match, 2)
		url := match[1]
		t.Logf("Found favicon reference at %s", url)

		resp, err := http.Get(queryAddr + "/" + url)
		require.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
	t.Run("React_app", func(t *testing.T) {
		assert.Contains(t, body, `<div id="jaeger-ui-root"></div>`)
	})
}

func createTrace(t *testing.T) {
	// Since all requests to query service are traces, creating a new trace
	// is simply a matter of querying one of the endpoints.
	resp, _ := httpGet(t, queryAddr+getServicesURL)
	traceResponse := resp.Header.Get("traceresponse")
	// Expecting: [version] [trace-id] [child-id] [trace-flags]
	parts := strings.Split(traceResponse, "-")
	require.Len(t, parts, 4, "traceResponse=%s", traceResponse)
	traceID = parts[1]
	t.Logf("Created trace %s", traceID)
}

type response struct {
	Data []*ui.Trace `json:"data"`
}

func getAPITrace(t *testing.T) {
	var queryResponse response
	for i := 0; i < 20; i++ {
		_, body := httpGet(t, queryAddr+getTraceURL+traceID)
		require.NoError(t, json.Unmarshal(body, &queryResponse))

		if len(queryResponse.Data) == 1 {
			break
		}
		t.Logf("Trace %s not found (yet)", traceID)
		time.Sleep(time.Second)
	}

	require.Len(t, queryResponse.Data, 1)
	require.Len(t, queryResponse.Data[0].Spans, 1)
}

func getSamplingStrategy(t *testing.T) {
	// TODO should we test refreshing the strategy file?

	r, body := httpGet(t, samplingAddr+getSamplingStrategyURL)
	t.Logf("Sampling strategy response: %s", string(body))
	require.EqualValues(t, http.StatusOK, r.StatusCode)

	var queryResponse api_v2.SamplingStrategyResponse
	require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(body), &queryResponse))

	assert.NotNil(t, queryResponse.ProbabilisticSampling)
	assert.InDelta(t, 1.0, queryResponse.ProbabilisticSampling.SamplingRate, 0.01)
}

func getServicesAPIV3(t *testing.T) {
	_, body := httpGet(t, queryAddr+getServicesAPIV3URL)

	var servicesResponse struct {
		Services []string
	}
	require.NoError(t, json.Unmarshal(body, &servicesResponse))
	require.Len(t, servicesResponse.Services, 1)
	assert.Contains(t, servicesResponse.Services[0], "jaeger")
}
