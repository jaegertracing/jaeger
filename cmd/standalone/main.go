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
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	basic "github.com/jaegertracing/jaeger/cmd/builder"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	collector "github.com/jaegertracing/jaeger/cmd/collector/app/builder"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// standalone/main is a standalone full-stack jaeger backend, backed by a memory store
func main() {
	if os.Getenv(storage.SpanStorageEnvVar) == "" {
		os.Setenv(storage.SpanStorageEnvVar, "memory") // other storage types default to SpanStorage
	}
	storageFactory, err := storage.NewFactory()
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}
	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-standalone",
		Short: "Jaeger all-in-one distribution with agent, collector and query in one process.",
		Long: `Jaeger all-in-one distribution with agent, collector and query. Use with caution this version
		 uses only in-memory database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := flags.TryLoadConfigFile(v)
			if err != nil {
				return err
			}

			sFlags := new(flags.SharedFlags).InitFromViper(v)
			logger, err := sFlags.NewLogger(zap.NewProductionConfig())
			if err != nil {
				return err
			}

			runtime.GOMAXPROCS(runtime.NumCPU())

			mBldr := new(pMetrics.Builder).InitFromViper(v)
			metricsFactory, err := mBldr.CreateMetricsFactory("jaeger-standalone")
			if err != nil {
				return errors.Wrap(err, "Cannot create metrics factory")
			}

			storageFactory.InitFromViper(v)
			if err := storageFactory.Initialize(metricsFactory, logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}
			spanReader, err := storageFactory.CreateSpanReader()
			if err != nil {
				logger.Fatal("Failed to create span reader", zap.Error(err))
			}
			spanWriter, err := storageFactory.CreateSpanWriter()
			if err != nil {
				logger.Fatal("Failed to create span writer", zap.Error(err))
			}
			dependencyReader, err := storageFactory.CreateDependencyReader()
			if err != nil {
				logger.Fatal("Failed to create dependency reader", zap.Error(err))
			}

			aOpts := new(agentApp.Builder).InitFromViper(v)
			cOpts := new(collector.CollectorOptions).InitFromViper(v)
			qOpts := new(queryApp.QueryOptions).InitFromViper(v)

			startAgent(aOpts, cOpts, logger, metricsFactory)
			startCollector(cOpts, spanWriter, logger, metricsFactory)
			startQuery(qOpts, spanReader, dependencyReader, logger, metricsFactory, mBldr)

			select {}
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		storageFactory.AddFlags,
		agentApp.AddFlags,
		collector.AddFlags,
		queryApp.AddFlags,
		pMetrics.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
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
	spanWriter spanstore.Writer,
	logger *zap.Logger,
	baseFactory metrics.Factory,
) {
	metricsFactory := baseFactory.Namespace("jaeger-collector", nil)

	spanBuilder, err := collector.NewSpanHandlerBuilder(
		cOpts,
		spanWriter,
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
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
		zHandler := zipkin.NewAPIHandler(zipkinSpansHandler)
		zHandler.RegisterRoutes(r)
		httpPortStr := ":" + strconv.Itoa(zipkinPort)
		logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", zipkinPort))

		if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
			logger.Fatal("Could not launch service", zap.Error(err))
		}
	}
}

func startQuery(
	qOpts *queryApp.QueryOptions,
	spanReader spanstore.Reader,
	depReader dependencystore.Reader,
	logger *zap.Logger,
	baseFactory metrics.Factory,
	metricsBuilder *pMetrics.Builder,
) {
	tracer, closer, err := jaegerClientConfig.Configuration{
		Sampler: &jaegerClientConfig.SamplerConfig{
			Type:  "const",
			Param: 1.0,
		},
		RPCMetrics: true,
	}.New("jaeger-query", jaegerClientConfig.Metrics(baseFactory))
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	defer closer.Close()
	apiHandler := queryApp.NewAPIHandler(
		spanReader,
		depReader,
		queryApp.HandlerOptions.Prefix(qOpts.Prefix),
		queryApp.HandlerOptions.Logger(logger),
		queryApp.HandlerOptions.Tracer(tracer))

	r := mux.NewRouter()
	apiHandler.RegisterRoutes(r)
	registerStaticHandler(r, logger, qOpts)

	if h := metricsBuilder.Handler(); h != nil {
		logger.Info("Registering metrics handler with jaeger-query HTTP server", zap.String("route", metricsBuilder.HTTPRoute))
		r.Handle(metricsBuilder.HTTPRoute, h)
	}

	portStr := ":" + strconv.Itoa(qOpts.Port)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	logger.Info("Starting jaeger-query HTTP server", zap.Int("port", qOpts.Port))
	if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
		logger.Fatal("Could not launch jaeger-query service", zap.Error(err))
	}
}

func registerStaticHandler(r *mux.Router, logger *zap.Logger, qOpts *queryApp.QueryOptions) {
	staticHandler, err := queryApp.NewStaticAssetsHandler(qOpts.StaticAssets, qOpts.UIConfig)
	if err != nil {
		logger.Fatal("Could not create static assets handler", zap.Error(err))
	}
	if staticHandler != nil {
		staticHandler.RegisterRoutes(r)
	} else {
		logger.Info("Static handler is not registered")
	}
}
