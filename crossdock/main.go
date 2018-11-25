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
	"sync"
	"time"

	"github.com/crossdock/crossdock-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/crossdock/services"
)

const (
	behaviorEndToEnd = "endtoend"

	collectorService = "Collector"
	queryService     = "Query"
)

var (
	logger, _ = zap.NewDevelopment()
)

type clientHandler struct {
	sync.RWMutex

	xHandler http.Handler

	// initialized is true if the client has finished initializing all the components required for the tests
	initialized bool
}

func main() {
	handler := &clientHandler{}
	go handler.initialize()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// when method is HEAD, report back with a 200 when ready to run tests
		if r.Method == "HEAD" {
			if !handler.isInitialized() {
				http.Error(w, "Client not ready", http.StatusServiceUnavailable)
			}
			return
		}
		handler.xHandler.ServeHTTP(w, r)
	})
	http.ListenAndServe(":8080", nil)
}

func (h *clientHandler) initialize() {
	httpHealthCheck(logger, queryService, "http://jaeger-query:16687")
	logger.Info("Query started")
	httpHealthCheck(logger, collectorService, "http://jaeger-collector:14269")
	logger.Info("Collector started")
	logger.Info("Waiting 5 seconds for services to initialize")
	time.Sleep(time.Second * 5)
	queryService := services.NewQueryService("http://jaeger-query:16686", logger)
	agentService := services.NewAgentService("http://jaeger-agent:5778", logger)

	traceHandler := services.NewTraceHandler(queryService, agentService, logger)
	h.Lock()
	defer h.Unlock()
	h.initialized = true

	behaviors := crossdock.Behaviors{
		behaviorEndToEnd: traceHandler.EndToEndTest,
	}
	h.xHandler = crossdock.Handler(behaviors, true)
}

func (h *clientHandler) isInitialized() bool {
	h.RLock()
	defer h.RUnlock()
	return h.initialized
}

func httpHealthCheck(logger *zap.Logger, service, healthURL string) {
	for i := 0; i < 240; i++ {
		res, err := http.Get(healthURL)
		if err == nil && res.StatusCode == 204 {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(time.Second)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}
