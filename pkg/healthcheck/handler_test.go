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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestStatusString(t *testing.T) {
	tests := map[Status]string{
		Unavailable: "unavailable",
		Ready:       "ready",
		Broken:      "broken",
		Status(-1):  "unknown",
	}
	for k, v := range tests {
		assert.Equal(t, v, k.String())
	}
}

func TestStatusSetGet(t *testing.T) {
	hc := New(Unavailable)
	assert.Equal(t, Unavailable, hc.Get())

	logger, logBuf := testutils.NewLogger()
	hc = New(Unavailable, Logger(logger))
	assert.Equal(t, Unavailable, hc.Get())

	hc.Ready()
	assert.Equal(t, Ready, hc.Get())
	assert.Equal(t, map[string]string{"level": "info", "msg": "Health Check state change", "status": "ready"}, logBuf.JSONLine(0))
}

func TestPortBusy(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	logger, logBuf := testutils.NewLogger()
	_, err = New(Unavailable, Logger(logger)).Serve(port)
	assert.Error(t, err)
	assert.Equal(t, "Health Check server failed to listen", logBuf.JSONLine(0)["msg"])
}

func TestServeHandler(t *testing.T) {
	hc, err := New(Ready).Serve(0)
	require.NoError(t, err)
	defer hc.Close()
}
