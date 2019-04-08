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
	"net/http"
	"net/http/httptest"
	"testing"
	"encoding/json"
	"io/ioutil"
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

	hr := getHealthCheckResponse(t, resp, )

	assert.Equal(t, "Server available", hr.StatusMsg)
	assert.Equal(t, hc.upTimeStats.UpTime, hr.upTimeStats.UpTime)
	assert.True(t, hc.upTimeStats.StartedAt.Equal(hr.upTimeStats.StartedAt))

	time.Sleep(300)
	hc.Set(Unavailable)

	resp, err = http.Get(server.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	hrNew := getHealthCheckResponse(t, resp)
	assert.Equal(t, hc.upTimeStats.UpTime, hrNew.upTimeStats.UpTime)

}
func getHealthCheckResponse(t *testing.T, resp *http.Response) healthCheckResponse {
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	var hr healthCheckResponse
	err = json.Unmarshal(body, &hr)
	require.NoError(t, err)
	return hr
}
