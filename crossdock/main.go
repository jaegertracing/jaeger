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
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/crossdock/crossdock-go"
	"github.com/gocql/gocql"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"
	"golang.org/x/net/context"

	"github.com/uber/jaeger/crossdock/services"
	"github.com/uber/jaeger/pkg/cassandra"
	gocqlw "github.com/uber/jaeger/pkg/cassandra/gocql"
)

const (
	behaviorEndToEnd = "endtoend"

	collectorService = "Collector"
	agentService     = "Agent"
	queryService     = "Query"

	cmdDir       = "/cmd/"
	collectorCmd = cmdDir + "jaeger-collector"
	agentCmd     = cmdDir + "jaeger-agent"
	queryCmd     = cmdDir + "jaeger-query"

	collectorHostPort = "localhost:14267"
	agentURL          = "http://test_driver:5778"
	queryServiceURL   = "http://127.0.0.1:16686"

	schema = "/scripts/schema.cql"

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
	startCollector(logger)
	logger.Info("Collector started")
	agentService := startAgent("", logger)
	logger.Info("Agent started")
	queryService := startQueryService("", logger)
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

// startCollector starts the jaeger collector as a background process.
func startCollector(logger *zap.Logger) {
	forkCmd(
		logger,
		collectorCmd,
		"--query.static-files=/ui/",
		"--cassandra.keyspace=jaeger",
		"--cassandra.servers=cassandra",
		"--collector.zipkin.http-port=9411",
	)
	tChannelHealthCheck(logger, collectorService, collectorHostPort)
}

// startQueryService initiates the query service as a background process.
func startQueryService(url string, logger *zap.Logger) services.QueryService {
	forkCmd(
		logger,
		queryCmd,
		"--cassandra.keyspace=jaeger",
		"--cassandra.servers=cassandra",
	)
	if url == "" {
		url = queryServiceURL
	}
	healthCheck(logger, queryService, url)
	return services.NewQueryService(url, logger)
}

// startAgent initializes the jaeger agent as a background process.
func startAgent(url string, logger *zap.Logger) services.AgentService {
	forkCmd(
		logger,
		agentCmd,
		"--collector.host-port=localhost:14267",
		"--processor.zipkin-compact.server-host-port=test_driver:5775",
		"--processor.jaeger-compact.server-host-port=test_driver:6831",
		"--processor.jaeger-binary.server-host-port=test_driver:6832",
	)
	if url == "" {
		url = agentURL
	}
	healthCheck(logger, agentService, agentURL)
	return services.NewAgentService(url, logger)
}

func forkCmd(logger *zap.Logger, cmd string, args ...string) {
	c := exec.Command(cmd, args...)

	fwdStream := func(name string, pipe func() (io.ReadCloser, error), dest *os.File) {
		stream, err := pipe()
		if err != nil {
			logger.Fatal("Error creating pipe for "+name, zap.String("cmd", cmd), zap.Error(err))
		}
		go func() {
			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				fmt.Fprintf(dest, "%s[%s] => %s\n", cmd, name, scanner.Text())
			}
		}()
	}

	fwdStream("stdout", c.StdoutPipe, os.Stdout)
	fwdStream("stderr", c.StderrPipe, os.Stderr)

	logger.Info("starting child process", zap.String("cmd", cmd))
	if err := c.Start(); err != nil {
		logger.Fatal("Failed to fork sub-command", zap.String("cmd", cmd), zap.Error(err))
	}
}

// InitializeStorage initializes cassandra instances.
func InitializeStorage(logger *zap.Logger) {
	session := initializeCassandra(logger, cassandraHost, 4)
	if session == nil {
		logger.Fatal("Failed to initialize cassandra session")
	}
	logger.Info("Initialized cassandra session")
	err := services.InitializeCassandraSchema(session, schema, jaegerKeyspace, logger)
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
