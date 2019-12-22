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
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	agentRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	agentGrpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	agentTchanRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
	"github.com/jaegertracing/jaeger/cmd/all-in-one/setupcontext"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	collector "github.com/jaegertracing/jaeger/cmd/collector/app/builder"
	"github.com/jaegertracing/jaeger/cmd/collector/app/grpcserver"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	ss "github.com/jaegertracing/jaeger/plugin/sampling/strategystore"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
	istorage "github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	sc "github.com/jaegertracing/jaeger/thrift-gen/sampling"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// all-in-one/main is a standalone full-stack jaeger backend, backed by a memory store
func main() {

	setupcontext.SetAllInOne()

	svc := flags.NewService(ports.CollectorAdminHTTP)

	if os.Getenv(storage.SpanStorageTypeEnvVar) == "" {
		os.Setenv(storage.SpanStorageTypeEnvVar, "memory") // other storage types default to SpanStorage
	}
	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}
	strategyStoreFactory, err := ss.NewFactory(ss.FactoryConfigFromEnv())
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy store factory: %v", err)
	}

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-all-in-one",
		Short: "Jaeger all-in-one distribution with agent, collector and query in one process.",
		Long: `Jaeger all-in-one distribution with agent, collector and query. Use with caution this version
by default uses only in-memory database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger                     // shortcut
			rootMetricsFactory := svc.MetricsFactory // shortcut
			metricsFactory := rootMetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			tracerCloser := initTracer(rootMetricsFactory, svc.Logger)

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

			strategyStoreFactory.InitFromViper(v)
			strategyStore := initSamplingStrategyStore(strategyStoreFactory, metricsFactory, logger)

			aOpts := new(agentApp.Builder).InitFromViper(v)
			repOpts := new(agentRep.Options).InitFromViper(v, logger)
			tchanBuilder := agentTchanRep.NewBuilder().InitFromViper(v, logger)
			grpcBuilder := agentGrpcRep.NewConnBuilder().InitFromViper(v)
			cOpts := new(collector.CollectorOptions).InitFromViper(v)
			qOpts := new(queryApp.QueryOptions).InitFromViper(v)

			collectorSrv := startCollector(cOpts, spanWriter, logger, metricsFactory, strategyStore, svc.HC())
			startAgent(aOpts, repOpts, tchanBuilder, grpcBuilder, cOpts, logger, metricsFactory)
			querySrv := startQuery(
				svc, qOpts, archiveOptions(storageFactory, logger),
				spanReader, dependencyReader,
				rootMetricsFactory, metricsFactory,
			)

			svc.RunAndThen(func() {
				collectorSrv.GracefulStop()
				querySrv.Close()
				if closer, ok := spanWriter.(io.Closer); ok {
					err := closer.Close()
					if err != nil {
						logger.Error("Failed to close span writer", zap.Error(err))
					}
				}
				tracerCloser.Close()
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		storageFactory.AddFlags,
		agentApp.AddFlags,
		agentRep.AddFlags,
		agentTchanRep.AddFlags,
		agentGrpcRep.AddFlags,
		collector.AddFlags,
		queryApp.AddFlags,
		strategyStoreFactory.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func startAgent(
	b *agentApp.Builder,
	repOpts *agentRep.Options,
	tchanBuilder *agentTchanRep.Builder,
	grpcBuilder *agentGrpcRep.ConnBuilder,
	cOpts *collector.CollectorOptions,
	logger *zap.Logger,
	baseFactory metrics.Factory,
) {
	metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "agent", Tags: nil})

	grpcBuilder.CollectorHostPorts = append(grpcBuilder.CollectorHostPorts, fmt.Sprintf("127.0.0.1:%d", cOpts.CollectorGRPCPort))
	cp, err := agentApp.CreateCollectorProxy(repOpts, tchanBuilder, grpcBuilder, logger, metricsFactory)
	if err != nil {
		logger.Fatal("Could not create collector proxy", zap.Error(err))
	}

	agent, err := b.CreateAgent(cp, logger, baseFactory)
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
	strategyStore strategystore.StrategyStore,
	hc *healthcheck.HealthCheck,
) *grpc.Server {
	metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "collector", Tags: nil})

	spanHandlerBuilder := &collector.SpanHandlerBuilder{
		SpanWriter:     spanWriter,
		CollectorOpts:  *cOpts,
		Logger:         logger,
		MetricsFactory: metricsFactory,
	}

	zipkinSpansHandler, jaegerBatchesHandler, grpcHandler := spanHandlerBuilder.BuildHandlers()

	{
		ch, err := tchannel.NewChannel("jaeger-collector", &tchannel.ChannelOptions{})
		if err != nil {
			logger.Fatal("Unable to create new TChannel", zap.Error(err))
		}
		server := thrift.NewServer(ch)
		batchHandler := collectorApp.NewTChannelHandler(jaegerBatchesHandler, zipkinSpansHandler)
		server.Register(jc.NewTChanCollectorServer(batchHandler))
		server.Register(zc.NewTChanZipkinCollectorServer(batchHandler))
		server.Register(sc.NewTChanSamplingManagerServer(sampling.NewHandler(strategyStore)))
		portStr := ":" + strconv.Itoa(cOpts.CollectorPort)
		listener, err := net.Listen("tcp", portStr)
		if err != nil {
			logger.Fatal("Unable to start listening on channel", zap.Error(err))
		}
		logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", cOpts.CollectorPort))
		logger.Warn("TChannel has been deprecated and will be removed in a future release")
		ch.Serve(listener)
	}

	server, err := startGRPCServer(cOpts.CollectorGRPCPort, grpcHandler, strategyStore, logger)
	if err != nil {
		logger.Fatal("Could not start gRPC collector", zap.Error(err))
	}

	{
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
			hc.Set(healthcheck.Unavailable)
		}()
	}
	return server
}

func startGRPCServer(
	port int,
	handler *collectorApp.GRPCHandler,
	samplingStore strategystore.StrategyStore,
	logger *zap.Logger,
) (*grpc.Server, error) {
	server := grpc.NewServer()
	_, err := grpcserver.StartGRPCCollector(port, server, handler, samplingStore, logger, func(err error) {
		logger.Fatal("gRPC collector failed", zap.Error(err))
	})
	if err != nil {
		return nil, err
	}
	return server, err
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
	svc *flags.Service,
	qOpts *queryApp.QueryOptions,
	queryOpts *querysvc.QueryServiceOptions,
	spanReader spanstore.Reader,
	depReader dependencystore.Reader,
	rootFactory metrics.Factory,
	baseFactory metrics.Factory,
) *queryApp.Server {
	spanReader = storageMetrics.NewReadMetricsDecorator(spanReader, baseFactory.Namespace(metrics.NSOptions{Name: "query"}))
	qs := querysvc.NewQueryService(spanReader, depReader, *queryOpts)
	server := queryApp.NewServer(svc, qs, qOpts, opentracing.GlobalTracer())
	if err := server.Start(); err != nil {
		svc.Logger.Fatal("Could not start jaeger-query service", zap.Error(err))
	}
	return server
}

func initSamplingStrategyStore(
	samplingStrategyStoreFactory *ss.Factory,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) strategystore.StrategyStore {
	if err := samplingStrategyStoreFactory.Initialize(metricsFactory, logger); err != nil {
		logger.Fatal("Failed to init sampling strategy store factory", zap.Error(err))
	}
	strategyStore, err := samplingStrategyStoreFactory.CreateStrategyStore()
	if err != nil {
		logger.Fatal("Failed to create sampling strategy store", zap.Error(err))
	}
	return strategyStore
}

func archiveOptions(storageFactory istorage.Factory, logger *zap.Logger) *querysvc.QueryServiceOptions {
	opts := &querysvc.QueryServiceOptions{}
	if !opts.InitArchiveStorage(storageFactory, logger) {
		logger.Info("Archive storage not initialized")
	}
	return opts
}

func initTracer(metricsFactory metrics.Factory, logger *zap.Logger) io.Closer {
	traceCfg := &jaegerClientConfig.Configuration{
		ServiceName: "jaeger-query",
		Sampler: &jaegerClientConfig.SamplerConfig{
			Type:  "const",
			Param: 1.0,
		},
		RPCMetrics: true,
	}
	traceCfg, err := traceCfg.FromEnv()
	if err != nil {
		logger.Fatal("Failed to read tracer configuration", zap.Error(err))
	}
	tracer, closer, err := traceCfg.NewTracer(
		jaegerClientConfig.Metrics(metricsFactory),
		jaegerClientConfig.Logger(jaegerClientZapLog.NewLogger(logger)),
	)
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	opentracing.SetGlobalTracer(tracer)
	return closer
}
