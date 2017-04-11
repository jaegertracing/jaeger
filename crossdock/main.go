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

package main

import (
	"net/http"
	"sync"

	"github.com/crossdock/crossdock-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/crossdock/services"
)

const (
	behaviorEndToEnd = "endtoend"
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
	services.InitializeStorage(logger)
	logger.Info("Cassandra started")
	services.InitializeCollector(logger)
	logger.Info("Collector started")
	agentService := services.InitializeAgent("", logger)
	logger.Info("Agent started")
	queryService := services.NewQueryService("", logger)
	logger.Info("Query started")
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
