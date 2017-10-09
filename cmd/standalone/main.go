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
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	agentApp "github.com/uber/jaeger/cmd/agent/app"
	basic "github.com/uber/jaeger/cmd/builder"
	collectorApp "github.com/uber/jaeger/cmd/collector/app"
	collector "github.com/uber/jaeger/cmd/collector/app/builder"
	"github.com/uber/jaeger/cmd/collector/app/zipkin"
	"github.com/uber/jaeger/cmd/flags"
	queryApp "github.com/uber/jaeger/cmd/query/app"
	query "github.com/uber/jaeger/cmd/query/app/builder"
	"github.com/uber/jaeger/pkg/config"
	pMetrics "github.com/uber/jaeger/pkg/metrics"
	"github.com/uber/jaeger/pkg/recoveryhandler"
	"github.com/uber/jaeger/storage/spanstore/memory"
	jc "github.com/uber/jaeger/thrift-gen/jaeger"
	zc "github.com/uber/jaeger/thrift-gen/zipkincore"
)

// standalone/main is a standalone full-stack jaeger backend, backed by a memory store
func main() {
	logger, _ := zap.NewProduction()
	v := viper.New()

	command := &cobra.Command{
		Use:   "jaeger-standalone",
		Short: "Jaeger all-in-one distribution with agent, collector and query in one process.",
		Long: `Jaeger all-in-one distribution with agent, collector and query. Use with caution this version
		 uses only in-memory database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if c := new(flags.ExternalConfFlags).InitFromViper(v); c.ConfigFile != "" {
				v.SetConfigFile(c.ConfigFile)
				err := v.ReadInConfig()
				if err != nil {
					logger.Fatal(fmt.Sprintf("Fatal error config file: %s \n", c.ConfigFile), zap.Error(err))
				}
			}

			runtime.GOMAXPROCS(runtime.NumCPU())
			sFlags := new(flags.SharedFlags).InitFromViper(v)
			cOpts := new(collector.CollectorOptions).InitFromViper(v)
			qOpts := new(query.QueryOptions).InitFromViper(v)

			metricsFactory := xkit.Wrap("jaeger-standalone", expvar.NewFactory(10))
			memStore := memory.NewStore()

			builder := &agentApp.Builder{}
			builder.InitFromViper(v)
			startAgent(builder, cOpts, logger, metricsFactory)
			startCollector(cOpts, sFlags, logger, metricsFactory, memStore)
			startQuery(qOpts, sFlags, logger, metricsFactory, memStore)
			select {}
		},
	}

	config.AddFlags(
		v,
		command,
		flags.AddConfFileFlag,
		flags.AddFlags,
		collector.AddFlags,
		query.AddFlags,
		agentApp.AddFlags,
		pMetrics.AddFlags,
	)

	if err := command.Execute(); err != nil {
		logger.Fatal("standalone command failed", zap.Error(err))
	}
}

func startAgent(
	b *agentApp.Builder,
	cOpts *collector.CollectorOptions,
	logger *zap.Logger,
	baseFactory metrics.Factory,
) {
	metricsFactory := baseFactory.Namespace("jaeger-agent", nil)

	if len(b.CollectorHostPorts) == 0 {
		b.CollectorHostPorts = append(b.CollectorHostPorts, fmt.Sprintf("127.0.0.1:%d", cOpts.CollectorPort))
	}
	agent, err := b.WithMetricsFactory(metricsFactory).CreateAgent(logger)
	if err != nil {
		logger.Fatal("Unable to initialize Jaeger Agent", zap.Error(err))
	}

	logger.Info("Starting agent")
	if err := agent.Run(); err != nil {
		logger.Fatal("Failed to run the agent", zap.Error(err))
	}
}

func startCollector(
	cOpts *collector.CollectorOptions,
	sFlags *flags.SharedFlags,
	logger *zap.Logger,
	baseFactory metrics.Factory,
	memoryStore *memory.Store,
) {
	metricsFactory := baseFactory.Namespace("jaeger-collector", nil)

	spanBuilder, err := collector.NewSpanHandlerBuilder(
		cOpts,
		sFlags,
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
		basic.Options.MemoryStoreOption(memoryStore),
	)
	if err != nil {
		logger.Fatal("Unable to set up builder", zap.Error(err))
	}
	ch, err := tchannel.NewChannel("jaeger-collector", &tchannel.ChannelOptions{})
	if err != nil {
		logger.Fatal("Unable to create new TChannel", zap.Error(err))
	}
	server := thrift.NewServer(ch)
	zipkinSpansHandler, jaegerBatchesHandler := spanBuilder.BuildHandlers()
	server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
	server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))
	portStr := ":" + strconv.Itoa(cOpts.CollectorPort)
	listener, err := net.Listen("tcp", portStr)
	if err != nil {
		logger.Fatal("Unable to start listening on channel", zap.Error(err))
	}
	ch.Serve(listener)
	logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", cOpts.CollectorPort))

	r := mux.NewRouter()
	apiHandler := collectorApp.NewAPIHandler(jaegerBatchesHandler)
	apiHandler.RegisterRoutes(r)
	httpPortStr := ":" + strconv.Itoa(cOpts.CollectorHTTPPort)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

	go startZipkinHTTPAPI(logger, cOpts.CollectorZipkinHTTPPort, zipkinSpansHandler, recoveryHandler)

	logger.Info("Starting jaeger-collector HTTP server", zap.Int("http-port", cOpts.CollectorHTTPPort))
	go func() {
		if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
			logger.Fatal("Could not launch jaeger-collector HTTP server", zap.Error(err))
		}
	}()
}

func startZipkinHTTPAPI(
	logger *zap.Logger,
	zipkinPort int,
	zipkinSpansHandler collectorApp.ZipkinSpansHandler,
	recoveryHandler func(http.Handler) http.Handler,
) {
	if zipkinPort != 0 {
		r := mux.NewRouter()
		zipkin.NewAPIHandler(zipkinSpansHandler).RegisterRoutes(r)
		httpPortStr := ":" + strconv.Itoa(zipkinPort)
		logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", zipkinPort))

		if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
			logger.Fatal("Could not launch service", zap.Error(err))
		}
	}
}

func startQuery(
	qOpts *query.QueryOptions,
	sFlags *flags.SharedFlags,
	logger *zap.Logger,
	baseFactory metrics.Factory,
	memoryStore *memory.Store,
) {
	metricsFactory := baseFactory.Namespace("jaeger-query", nil)

	storageBuild, err := query.NewStorageBuilder(
		sFlags.SpanStorage.Type,
		sFlags.DependencyStorage.DataFrequency,
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
		basic.Options.MemoryStoreOption(memoryStore),
	)
	if err != nil {
		logger.Fatal("Failed to wire up service", zap.Error(err))
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
		storageBuild.SpanReader,
		storageBuild.DependencyReader,
		queryApp.HandlerOptions.Prefix(qOpts.QueryPrefix),
		queryApp.HandlerOptions.Logger(logger),
		queryApp.HandlerOptions.Tracer(tracer))
	sHandler := queryApp.NewStaticAssetsHandler(qOpts.QueryStaticAssets)
	r := mux.NewRouter()
	rHandler.RegisterRoutes(r)
	sHandler.RegisterRoutes(r)
	portStr := ":" + strconv.Itoa(qOpts.QueryPort)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	logger.Info("Starting jaeger-query HTTP server", zap.Int("port", qOpts.QueryPort))
	if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
		logger.Fatal("Could not launch jaeger-query service", zap.Error(err))
	}
}
