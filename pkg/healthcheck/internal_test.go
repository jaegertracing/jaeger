// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
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

package healthcheck

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHttpCall(t *testing.T) {
	hc := New()
	handler := hc.Handler()

	server := httptest.NewServer(handler)
	defer server.Close()

	hc.Set(Ready)

	resp, err := http.Get(server.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	hr := parseHealthCheckResponse(t, resp)
	assert.Equal(t, "Server available", hr.StatusMsg)
	// de-serialized timestamp loses monotonic clock value, so assert.Equals() doesn't work.
	// https://github.com/stretchr/testify/issues/502
	if want, have := hc.getState().upSince, hr.UpSince; !assert.True(t, want.Equal(have)) {
		t.Logf("want='%v', have='%v'", want, have)
	}
	assert.NotZero(t, hr.Uptime)
	t.Logf("uptime=%v", hr.Uptime)

	time.Sleep(time.Millisecond)
	hc.Set(Unavailable)

	resp, err = http.Get(server.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	hrNew := parseHealthCheckResponse(t, resp)
	assert.Zero(t, hrNew.Uptime)
	assert.Zero(t, hrNew.UpSince)
}

func parseHealthCheckResponse(t *testing.T, resp *http.Response) healthCheckResponse {
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	var hr healthCheckResponse
	err = json.Unmarshal(body, &hr)
	require.NoError(t, err)
	return hr
}
