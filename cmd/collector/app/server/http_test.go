// Copyright (c) 2020 The Jaeger Authors.
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

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

// test wrong port number
func TestFailToListenHTTP(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	server, err := StartHTTPServer(&HTTPServerParams{
		HostPort: ":-1",
		Logger:   logger,
	})
	assert.Nil(t, server)
	assert.EqualError(t, err, "listen tcp: address -1: invalid port")
}

func TestSpanCollectorHTTP(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	params := &HTTPServerParams{
		Handler:        handler.NewJaegerSpanHandler(logger, &mockSpanProcessor{}),
		SamplingStore:  &mockSamplingStore{},
		MetricsFactory: metricstest.NewFactory(time.Hour),
		HealthCheck:    healthcheck.New(),
		Logger:         logger,
	}

	server := httptest.NewServer(nil)
	defer server.Close()

	serveHTTP(server.Config, server.Listener, params)

	response, err := http.Post(server.URL, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, response)
}
