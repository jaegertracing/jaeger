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

package healthcheck_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger/pkg/healthcheck"
	"go.uber.org/zap"
)

func TestProperInitialState(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	s, err := healthcheck.NewState(http.StatusServiceUnavailable, logger)
	if err != nil {
		t.Error("Could not start the health check server.", zap.Error(err))
	}

	if http.StatusServiceUnavailable != s.Get() {
		t.Errorf("Expected another handler state. Is: %d, expected: %d", s.Get(), http.StatusServiceUnavailable)
	}
}

func TestChangedState(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	s, err := healthcheck.NewState(http.StatusServiceUnavailable, logger)
	if err != nil {
		t.Error("Could not start the health check server.", zap.Error(err))
	}
	s.Set(http.StatusInternalServerError)

	if http.StatusInternalServerError != s.Get() {
		t.Errorf("Expected another handler state. Is: %d, expected: %d", s.Get(), http.StatusInternalServerError)
	}
}

func TestStateIsReady(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	s, err := healthcheck.NewState(http.StatusServiceUnavailable, logger)
	if err != nil {
		t.Error("Could not start the health check server.", zap.Error(err))
	}
	s.Ready()

	if http.StatusNoContent != s.Get() {
		t.Errorf("Expected another handler state. Is: %d, expected: %d", s.Get(), http.StatusNoContent)
	}
}

func TestPortBusy(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	l, err := net.Listen("tcp", ":0")
	defer l.Close()
	if err != nil {
		t.Error("Could not start on a random port.", zap.Error(err))
	}
	port := l.Addr().(*net.TCPAddr).Port

	_, err = healthcheck.Serve(http.StatusServiceUnavailable, port, logger)
	if err == nil {
		t.Error("We expected an error on trying to serve on a non-free port.", zap.Error(err))
	}
}

func TestHttpCall(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	state, err := healthcheck.NewState(http.StatusServiceUnavailable, logger)
	handler, err := healthcheck.NewHandler(state)
	if err != nil {
		t.Error("Could not start the health check server.", zap.Error(err))
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	state.Ready()

	resp, err := http.Get(server.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestListenerClose(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	state, err := healthcheck.NewState(http.StatusServiceUnavailable, logger)
	handler, err := healthcheck.NewHandler(state)
	if err != nil {
		t.Error("Could not start the health check server.", zap.Error(err))
	}

	s := &http.Server{Handler: handler}
	l, err := net.Listen("tcp", ":0")
	defer l.Close()
	s, err = healthcheck.ServeWithListener(l, s, logger)
}

func TestServeHandler(t *testing.T) {
	healthcheck.Serve(http.StatusServiceUnavailable, 0, zap.NewNop())
}
