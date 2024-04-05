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

package main

import (
	"bytes"
	"encoding/json"
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

	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// These tests are only run when the environment variable TEST_MODE=integration is set.
// An optional SKIP_SAMPLING=true environment variable can be used to skip sampling checks (for jaeger-v2).

const (
	host      = "0.0.0.0"
	queryPort = "16686"
	agentPort = "5778"
	queryAddr = "http://" + host + ":" + queryPort
	agentAddr = "http://" + host + ":" + agentPort

	getServicesURL         = "/api/services"
	getTraceURL            = "/api/traces/"
	getServicesAPIV3URL    = "/api/v3/services"
	getSamplingStrategyURL = "/sampling?service=whatever"
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

	t.Run("checkWebUI", checkWebUI)
	t.Run("createTrace", createTrace)
	t.Run("getAPITrace", getAPITrace)
	t.Run("getSamplingStrategy", getSamplingStrategy)
	t.Run("getServicesAPIV3", getServicesAPIV3)
}

func healthCheck(t *testing.T) {
	require.Eventuallyf(
		t,
		func() bool {
			_, err := http.Get(queryAddr + "/")
			return err == nil
		},
		10*time.Second,
		time.Second,
		"expecting query endpoint to be healhty",
	)
	t.Logf("Server detected at %s", queryAddr)
}

func checkWebUI(t *testing.T) {
	resp, err := http.Get(queryAddr + "/")
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
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
	req, err := http.NewRequest(http.MethodGet, queryAddr+getServicesURL, nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
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
	req, err := http.NewRequest(http.MethodGet, queryAddr+getTraceURL+traceID, nil)
	require.NoError(t, err)

	var queryResponse response

	for i := 0; i < 20; i++ {
		resp, err := httpClient.Do(req)
		require.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)

		err = json.Unmarshal(body, &queryResponse)
		require.NoError(t, err)
		resp.Body.Close()

		if len(queryResponse.Data) == 1 {
			break
		}
		time.Sleep(time.Second)
	}
	require.Len(t, queryResponse.Data, 1)
	require.Len(t, queryResponse.Data[0].Spans, 1)
}

func getSamplingStrategy(t *testing.T) {
	// TODO once jaeger-v2 can pass this test, remove from .github/workflows/ci-all-in-one-build.yml
	if os.Getenv("SKIP_SAMPLING") == "true" {
		t.Skip("skipping sampling strategy check because SKIP_SAMPLING=true is set")
	}
	req, err := http.NewRequest(http.MethodGet, agentAddr+getSamplingStrategyURL, nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)

	var queryResponse api_v2.SamplingStrategyResponse
	err = jsonpb.Unmarshal(bytes.NewReader(body), &queryResponse)
	require.NoError(t, err)
	resp.Body.Close()

	assert.NotNil(t, queryResponse.ProbabilisticSampling)
	assert.EqualValues(t, 1.0, queryResponse.ProbabilisticSampling.SamplingRate)
}

func getServicesAPIV3(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, queryAddr+getServicesAPIV3URL, nil)
	require.NoError(t, err)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)

	var servicesResponse struct {
		Services []string
	}
	err = json.Unmarshal(body, &servicesResponse)
	require.NoError(t, err)
	require.Len(t, servicesResponse.Services, 1)
	assert.Contains(t, servicesResponse.Services[0], "jaeger")
}
