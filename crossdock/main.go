package main

import (
	"net/http"
	"sync"

	"github.com/crossdock/crossdock-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/crossdock/services"
)

const (
	behaviorEndToEnd         = "endtoend"
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
		behaviorEndToEnd:         traceHandler.EndToEndTest,
	}
	h.xHandler = crossdock.Handler(behaviors, true)
}

func (h *clientHandler) isInitialized() bool {
	h.RLock()
	defer h.RUnlock()
	return h.initialized
}
