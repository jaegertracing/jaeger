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
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/crossdock/crossdock-go"
	"github.com/gocql/gocql"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/crossdock/services"

	"golang.org/x/net/context"

	"github.com/uber/jaeger/pkg/cassandra"
	gocqlw "github.com/uber/jaeger/pkg/cassandra/gocql"
)

const (
	behaviorEndToEnd = "endtoend"

	collectorService = "Collector"
	agentService     = "Agent"
	queryService     = "Query"

	cmdDir       = "/go/cmd/"
	collectorCmd = cmdDir + "jaeger-collector %s &"
	agentCmd     = cmdDir + "jaeger-agent %s &"
	queryCmd     = cmdDir + "jaeger-query %s &"

	collectorHostPort = "localhost:14267"
	agentURL          = "http://test_driver:5778"
	queryServiceURL   = "http://127.0.0.1:16686"

	scriptsDir = "/go/scripts/"
	schema     = scriptsDir + "schema.cql"

	jaegerKeyspace = "jaeger"

	cassandraHost = "cassandra"
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
	InitializeStorage(logger)
	logger.Info("Cassandra started")
	InitializeCollector(logger)
	logger.Info("Collector started")
	agentService := InitializeAgent("", logger)
	logger.Info("Agent started")
	queryService := NewQueryService("", logger)
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

// InitializeCollector initializes the jaeger collector
func InitializeCollector(logger *zap.Logger) {
	cmd := exec.Command("/bin/bash", "-c",
		fmt.Sprintf(collectorCmd, "-cassandra.keyspace=jaeger -cassandra.servers=cassandra -cassandra.connections-per-host=1"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize collector service", zap.Error(err))
	}
	tChannelHealthCheck(logger, collectorService, collectorHostPort)
}

// NewQueryService initiates the query service
func NewQueryService(url string, logger *zap.Logger) services.QueryService {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf(queryCmd,
		"-cassandra.keyspace=jaeger -cassandra.servers=cassandra -cassandra.connections-per-host=1"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize query service", zap.Error(err))
	}
	if url == "" {
		url = queryServiceURL
	}
	healthCheck(logger, queryService, url)
	return services.NewQueryService(url, logger)
}

// InitializeAgent initializes the jaeger agent.
func InitializeAgent(url string, logger *zap.Logger) services.AgentService {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf(agentCmd,
		"-collector.host-port=localhost:14267 -processor.zipkin-compact.server-host-port=test_driver:5775 "+
			"-processor.jaeger-compact.server-host-port=test_driver:6831 -processor.jaeger-binary.server-host-port=test_driver:6832"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize agent service", zap.Error(err))
	}
	if url == "" {
		url = agentURL
	}
	healthCheck(logger, agentService, agentURL)
	return services.NewAgentService(url, logger)
}

// InitializeStorage initializes cassandra instances.
func InitializeStorage(logger *zap.Logger) {
	session := initializeCassandra(logger, cassandraHost, 4)
	if session == nil {
		logger.Fatal("Failed to initialize cassandra session")
	}
	logger.Info("Initialized cassandra session")
	err := services.InitializeCassandraSchema(session, schema, jaegerKeyspace)
	if err != nil {
		logger.Fatal("Could not initialize cassandra schema", zap.Error(err))
	}
}

func newCassandraCluster(host string, protoVersion int) (cassandra.Session, error) {
	cluster := gocql.NewCluster(host)
	cluster.ProtoVersion = protoVersion
	cluster.Timeout = 30 * time.Second
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}
	return gocqlw.WrapCQLSession(session), nil
}

func initializeCassandra(logger *zap.Logger, host string, protoVersion int) cassandra.Session {
	var session cassandra.Session
	var err error
	for i := 0; i < 60; i++ {
		session, err = newCassandraCluster(host, protoVersion)
		if err == nil {
			break
		}
		logger.Warn("Failed to initialize cassandra session", zap.String("host", host), zap.Error(err))
		time.Sleep(1 * time.Second)
	}
	return session
}

func healthCheck(logger *zap.Logger, service, healthURL string) {
	for i := 0; i < 100; i++ {
		_, err := http.Get(healthURL)
		if err == nil {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}

func tChannelHealthCheck(logger *zap.Logger, service, hostPort string) {
	channel, _ := tchannel.NewChannel("test_driver", nil)
	for i := 0; i < 100; i++ {
		err := channel.Ping(context.Background(), hostPort)
		if err == nil {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}
