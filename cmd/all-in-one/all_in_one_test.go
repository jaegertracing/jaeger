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

//go:build integration
// +build integration

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
)

const (
	host          = "0.0.0.0"
	queryPort     = "16686"
	agentPort     = "5778"
	queryHostPort = host + ":" + queryPort
	queryURL      = "http://" + queryHostPort
	agentHostPort = host + ":" + agentPort
	agentURL      = "http://" + agentHostPort

	getServicesURL         = queryURL + "/api/services"
	getSamplingStrategyURL = agentURL + "/sampling?service=whatever"

	getServicesAPIV3URL = queryURL + "/api/v3/services"
)

var getTraceURL = queryURL + "/api/traces/"

var httpClient = &http.Client{
	Timeout: time.Second,
}

func TestAllInOne(t *testing.T) {
	// Check if the query service is available
	healthCheck(t)

	t.Run("Check if the favicon icon is available", jaegerLogoCheck)
	t.Run("createTrace", createTrace)
	t.Run("getAPITrace", getAPITrace)
	t.Run("getSamplingStrategy", getSamplingStrategy)
	t.Run("getServicesAPIV3", getServicesAPIV3)
}

func createTrace(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, getServicesURL, nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	traceResponse := resp.Header.Get("traceresponse")
	parts := strings.Split(traceResponse, "-")
	require.Len(t, parts, 4) // [version] [trace-id] [child-id] [trace-flags]
	traceID := parts[1]
	getTraceURL += traceID
}

type response struct {
	Data []*ui.Trace `json:"data"`
}

func getAPITrace(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, getTraceURL, nil)
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
	req, err := http.NewRequest(http.MethodGet, getSamplingStrategyURL, nil)
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

func healthCheck(t *testing.T) {
	t.Log("Health-checking all-in-one...")
	require.Eventuallyf(
		t,
		func() bool {
			_, err := http.Get(queryURL)
			return err == nil
		},
		10*time.Second,
		time.Second,
		"expecting query endpoint to be healhty",
	)
}

func jaegerLogoCheck(t *testing.T) {
	t.Log("Checking favicon...")
	resp, err := http.Get(queryURL + "/static/jaeger-logo-ab11f618.svg")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func getServicesAPIV3(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, getServicesAPIV3URL, nil)
	require.NoError(t, err)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)

	var servicesResponse api_v3.GetServicesResponse
	jsonpb := runtime.JSONPb{}
	err = jsonpb.Unmarshal(body, &servicesResponse)
	require.NoError(t, err)
	assert.Equal(t, []string{"jaeger-all-in-one"}, servicesResponse.GetServices())
}
