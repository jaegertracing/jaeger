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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestHttpCall(t *testing.T) {
	hc := New(Unavailable)
	handler := hc.httpHandler()

	server := httptest.NewServer(handler)
	defer server.Close()

	hc.Set(Ready)

	resp, err := http.Get(server.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestListenerClose(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	hc := New(Unavailable, Logger(logger))

	l, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)
	l.Close()

	hc.serveWithListener(l)
	for i := 0; i < 1000; i++ {
		if hc.Get() == Broken {
			break
		}
		time.Sleep(time.Millisecond)
	}
	assert.Equal(t, Broken, hc.Get())
	log := logBuf.JSONLine(0)
	assert.Equal(t, "failed to serve", log["msg"])
}
