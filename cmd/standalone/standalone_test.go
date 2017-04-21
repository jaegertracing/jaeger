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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"code.uber.internal/infra/jaeger/model/ui"
)

const (
	host          = "0.0.0.0"
	queryPort     = "16686"
	queryHostPort = host + ":" + queryPort
	queryURL      = "http://" + queryHostPort

	getServicesURL = queryURL + "/api/services"
	getTraceURL    = queryURL + "/api/traces?service=jaeger-query&tag=jaeger-debug-id:debug"
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

func healthCheck() error {
	for i := 0; i < 100; i++ {
		if _, err := http.Get(queryURL); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("query service is not ready")
}
