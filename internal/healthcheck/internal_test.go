// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package healthcheck

import (
	"encoding/json"
	"io"
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
	assert.NotEmpty(t, hr.Uptime)
	t.Logf("uptime=%v", hr.Uptime)

	time.Sleep(time.Millisecond)
	hc.Set(Unavailable)

	resp, err = http.Get(server.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	hrNew := parseHealthCheckResponse(t, resp)
	assert.Empty(t, hrNew.Uptime)
	assert.Zero(t, hrNew.UpSince)
}

func parseHealthCheckResponse(t *testing.T, resp *http.Response) healthCheckResponse {
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var hr healthCheckResponse
	err = json.Unmarshal(body, &hr)
	require.NoError(t, err)
	return hr
}
