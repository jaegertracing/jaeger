// Copyright (c) 2019 The Jaeger Authors.
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

package main

import (
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/crossdock/crossdock-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/crossdock/services"
)

const (
	behaviorEndToEnd = "endtoend"

	envAgentHostPort = "JAEGER_AGENT_HOST_PORT"
	envQueryHostPort = "JAEGER_QUERY_HOST_PORT"

	envQueryHealthcheckHostPort     = "JAEGER_QUERY_HC_HOST_PORT"
	envCollectorHealthcheckHostPort = "JAEGER_COLLECTOR_HC_HOST_PORT"
)

var (
	logger, _ = zap.NewDevelopment()

	agentHostPort string
	queryHostPort string

	queryHealthcheckHostPort     string
	collectorHealthcheckHostPort string
)

type clientHandler struct {
	// initialized (atomic) is non-zero all components required for the tests are available
	initialized uint64

	xHandler http.Handler
}

func main() {
	agentHostPort = getEnv(envAgentHostPort, "jaeger-agent:5778")
	queryHostPort = getEnv(envQueryHostPort, "jaeger-query:16686")
	queryHealthcheckHostPort = getEnv(envQueryHealthcheckHostPort, "jaeger-query:16687")
	collectorHealthcheckHostPort = getEnv(envCollectorHealthcheckHostPort, "jaeger-collector:14269")

	handler := &clientHandler{}
	go handler.initialize()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// when method is HEAD, report back with a 200 when ready to run tests
		if r.Method == "HEAD" {
			if !handler.isInitialized() {
				http.Error(w, "Components not ready", http.StatusServiceUnavailable)
			}
			return
		}
		handler.xHandler.ServeHTTP(w, r)
	})
	http.ListenAndServe(":8080", nil)
}

func getEnv(key string, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}

func (h *clientHandler) initialize() {
	httpHealthCheck(logger, "jaeger-query", "http://"+queryHealthcheckHostPort)
	httpHealthCheck(logger, "jaeger-collector", "http://"+collectorHealthcheckHostPort)

	queryService := services.NewQueryService("http://"+queryHostPort, logger)
	agentService := services.NewAgentService("http://"+agentHostPort, logger)

	traceHandler := services.NewTraceHandler(queryService, agentService, logger)
	behaviors := crossdock.Behaviors{
		behaviorEndToEnd: traceHandler.EndToEndTest,
	}
	h.xHandler = crossdock.Handler(behaviors, true)

	atomic.StoreUint64(&h.initialized, 1)
}

func (h *clientHandler) isInitialized() bool {
	return atomic.LoadUint64(&h.initialized) != 0
}

func is2xxStatusCode(statusCode int) bool {
	return statusCode >= 200 && statusCode <= 299
}

func httpHealthCheck(logger *zap.Logger, service, healthURL string) {
	for i := 0; i < 240; i++ {
		res, err := http.Get(healthURL)
		if err == nil && is2xxStatusCode(res.StatusCode) {
			logger.Info("Health check successful", zap.String("service", service))
			return
		}
		logger.Info("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(time.Second)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}
