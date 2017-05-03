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
	"flag"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"

	"github.com/gorilla/mux"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	agentApp "github.com/uber/jaeger/cmd/agent/app"
	basic "github.com/uber/jaeger/cmd/builder"
	collector "github.com/uber/jaeger/cmd/collector/app/builder"
	queryApp "github.com/uber/jaeger/cmd/query/app"
	query "github.com/uber/jaeger/cmd/query/app/builder"
	"github.com/uber/jaeger/pkg/recoveryhandler"
	"github.com/uber/jaeger/storage/spanstore/memory"
	jc "github.com/uber/jaeger/thrift-gen/jaeger"
	zc "github.com/uber/jaeger/thrift-gen/zipkincore"
)

// standalone/main is a standalone full-stack jaeger backend, backed by a memory store
func main() {
	logger, _ := zap.NewProduction()
	metricsFactory := xkit.Wrap("jaeger-standalone", expvar.NewFactory(10))
	memStore := memory.NewStore()

	builder := agentApp.NewBuilder()
	builder.Bind(flag.CommandLine)
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	startAgent(logger, metricsFactory, builder)
	startCollector(logger, metricsFactory, memStore)
	startQuery(logger, metricsFactory, memStore)

	select {}
}

func startAgent(logger *zap.Logger, baseFactory metrics.Factory, builder *agentApp.Builder) {
	metricsFactory := baseFactory.Namespace("jaeger-agent", nil)

	if builder.CollectorHostPort == "" {
		builder.CollectorHostPort = fmt.Sprintf("127.0.0.1:%d", *collector.CollectorPort)
	}
	agent, err := builder.CreateAgent(metricsFactory, logger)
	if err != nil {
		logger.Fatal("Unable to initialize Jaeger Agent", zap.Error(err))
	}

	logger.Info("Starting agent")
	if err := agent.Run(); err != nil {
		logger.Fatal("Failed to run the agent", zap.Error(err))
	}
}

func startCollector(logger *zap.Logger, baseFactory metrics.Factory, memoryStore *memory.Store) {
	metricsFactory := baseFactory.Namespace("jaeger-collector", nil)

	spanBuilder, err := collector.NewSpanHandlerBuilder(
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
		basic.Options.MemoryStoreOption(memoryStore),
	)
	if err != nil {
		logger.Fatal("Unable to set up builder", zap.Error(err))
	}
	zipkinSpansHandler, jaegerBatchesHandler, err := spanBuilder.BuildHandlers()
	if err != nil {
		logger.Fatal("Unable to build span handlers", zap.Error(err))
	}

	ch, err := tchannel.NewChannel("jaeger-collector", &tchannel.ChannelOptions{})
	if err != nil {
		logger.Fatal("Unable to create new TChannel", zap.Error(err))
	}
	server := thrift.NewServer(ch)
	server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
	server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))
	portStr := ":" + strconv.Itoa(*collector.CollectorPort)
	listener, err := net.Listen("tcp", portStr)
	if err != nil {
		logger.Fatal("Unable to start listening on channel", zap.Error(err))
	}
	ch.Serve(listener)
	logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", *collector.CollectorPort))
}

func startQuery(logger *zap.Logger, baseFactory metrics.Factory, memoryStore *memory.Store) {
	metricsFactory := baseFactory.Namespace("jaeger-query", nil)

	storageBuild, err := query.NewStorageBuilder(
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
		basic.Options.MemoryStoreOption(memoryStore),
	)
	if err != nil {
		logger.Fatal("Failed to wire up service", zap.Error(err))
	}
	spanReader, err := storageBuild.NewSpanReader()
	if err != nil {
		logger.Fatal("Failed to get span reader", zap.Error(err))
	}
	dependencyReader, err := storageBuild.NewDependencyReader()
	if err != nil {
		logger.Fatal("Failed to get dependency reader", zap.Error(err))
	}
	tracer, closer, err := jaegerClientConfig.Configuration{
		Sampler: &jaegerClientConfig.SamplerConfig{
			Type:  "probabilistic",
			Param: 0.001,
		},
		RPCMetrics: true,
	}.New("jaeger-query", jaegerClientConfig.Metrics(baseFactory))
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	defer closer.Close()
	rHandler := queryApp.NewAPIHandler(
		spanReader,
		dependencyReader,
		queryApp.HandlerOptions.Prefix(*query.QueryPrefix),
		queryApp.HandlerOptions.Logger(logger),
		queryApp.HandlerOptions.Tracer(tracer))
	sHandler := queryApp.NewStaticAssetsHandler(*query.QueryStaticAssets)
	r := mux.NewRouter()
	rHandler.RegisterRoutes(r)
	sHandler.RegisterRoutes(r)
	portStr := ":" + strconv.Itoa(*query.QueryPort)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	logger.Info("Starting jaeger-query HTTP server", zap.Int("port", *query.QueryPort))
	if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
		logger.Fatal("Could not launch jaeger-query service", zap.Error(err))
	}
}
