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
