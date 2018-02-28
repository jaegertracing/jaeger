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

// +build integration

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
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
	getTraceURL            = queryURL + "/api/traces?service=jaeger-query&tag=jaeger-debug-id:debug"
	getSamplingStrategyURL = agentURL + "?serivce=whatever"
)

var (
	httpClient = &http.Client{
		Timeout: time.Second,
	}
)

func TestStandalone(t *testing.T) {
	// Check if the query service is available
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	createTrace(t)
	getAPITrace(t)
	getSamplingStrategy(t)
}

func createTrace(t *testing.T) {
	req, err := http.NewRequest("GET", getServicesURL, nil)
	require.NoError(t, err)
	req.Header.Add("jaeger-debug-id", "debug")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

type response struct {
	Data []*ui.Trace `json:"data"`
}

func getAPITrace(t *testing.T) {
	req, err := http.NewRequest("GET", getTraceURL, nil)
	require.NoError(t, err)

	var queryResponse response

	for i := 0; i < 20; i++ {
		resp, err := httpClient.Do(req)
		require.NoError(t, err)

		body, _ := ioutil.ReadAll(resp.Body)

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
	req, err := http.NewRequest("GET", getSamplingStrategyURL, nil)
	require.NoError(t, err)

	var queryResponse sampling.SamplingStrategyResponse
	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &queryResponse)
	require.NoError(t, err)
	resp.Body.Close()

	assert.NotNil(t, queryResponse.ProbabilisticSampling)
	assert.EqualValues(t, 1.0, queryResponse.ProbabilisticSampling.SamplingRate)
}

func healthCheck() error {
	for i := 0; i < 100; i++ {
		if _, err := http.Get(queryURL); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("query service is not ready")
}
