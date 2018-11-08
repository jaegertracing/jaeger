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
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	agentRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	agentGrpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	agentTchanRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
	basic "github.com/jaegertracing/jaeger/cmd/builder"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	collector "github.com/jaegertracing/jaeger/cmd/collector/app/builder"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	ss "github.com/jaegertracing/jaeger/plugin/sampling/strategystore"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	sc "github.com/jaegertracing/jaeger/thrift-gen/sampling"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// all-in-one/main is a standalone full-stack jaeger backend, backed by a memory store
func main() {
	var signalsChannel = make(chan os.Signal)
	signal.Notify(signalsChannel, os.Interrupt, syscall.SIGTERM)

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
			hc, err := sFlags.NewHealthCheck(logger)
			if err != nil {
				logger.Fatal("Could not start the health check server.", zap.Error(err))
			}

			mBldr := new(pMetrics.Builder).InitFromViper(v)
			metricsFactory, err := mBldr.CreateMetricsFactory("jaeger")
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

			strategyStore := initializeStrategyStore(strategyStoreFactory, v, metricsFactory, logger)

			aOpts := new(agentApp.Builder).InitFromViper(v)
			repOpts := new(agentRep.Options).InitFromViper(v)
			tchannelRepOpts := agentTchanRep.NewBuilder().InitFromViper(v, logger)
			grpcRepOpts := new(agentGrpcRep.Options).InitFromViper(v)
			cOpts := new(collector.CollectorOptions).InitFromViper(v)
			qOpts := new(queryApp.QueryOptions).InitFromViper(v)

			startAgent(aOpts, repOpts, tchannelRepOpts, grpcRepOpts, cOpts, logger, metricsFactory)
			startCollector(cOpts, spanWriter, logger, metricsFactory, strategyStore, hc)
			startQuery(qOpts, spanReader, dependencyReader, logger, metricsFactory, mBldr, hc)
			hc.Ready()
			<-signalsChannel
			logger.Info("Shutting down")
			if closer, ok := spanWriter.(io.Closer); ok {
				err := closer.Close()
				if err != nil {
					logger.Error("Failed to close span writer", zap.Error(err))
				}
			}
			logger.Info("Shutdown complete")
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())

	flags.SetDefaultHealthCheckPort(collector.CollectorDefaultHealthCheckHTTPPort)

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		storageFactory.AddFlags,
		agentApp.AddFlags,
		agentRep.AddFlags,
		agentTchanRep.AddFlags,
		agentGrpcRep.AddFlags,
		collector.AddFlags,
		queryApp.AddFlags,
		pMetrics.AddFlags,
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
	tchanRep *agentTchanRep.Builder,
	grpcRepOpts *agentGrpcRep.Options,
	cOpts *collector.CollectorOptions,
	logger *zap.Logger,
	baseFactory metrics.Factory,
) {
	metricsFactory := baseFactory.Namespace("agent", nil)

	cp, err := createCollectorProxy(cOpts, repOpts, tchanRep, grpcRepOpts, logger, metricsFactory)
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

func createCollectorProxy(
	cOpts *collector.CollectorOptions,
	opts *agentRep.Options,
	tchanRep *agentTchanRep.Builder,
	grpcRepOpts *agentGrpcRep.Options,
	logger *zap.Logger,
	mFactory metrics.Factory,
) (agentApp.CollectorProxy, error) {
	switch opts.ReporterType {
	case agentRep.GRPC:
		grpcRepOpts.CollectorHostPort = fmt.Sprintf("127.0.0.1:%d", cOpts.CollectorGRPCPort)
		return agentGrpcRep.NewCollectorProxy(grpcRepOpts, logger), nil
	default:
		logger.Warn("Specified unknown reporter type, falling back to tchannel")
		fallthrough
	case agentRep.TCHANNEL:
		tchanRep.CollectorHostPorts = append(tchanRep.CollectorHostPorts, fmt.Sprintf("127.0.0.1:%d", cOpts.CollectorPort))
		return agentTchanRep.NewCollectorProxy(tchanRep, mFactory, logger)
	}
}

func startCollector(
	cOpts *collector.CollectorOptions,
	spanWriter spanstore.Writer,
	logger *zap.Logger,
	baseFactory metrics.Factory,
	strategyStore strategystore.StrategyStore,
	hc *healthcheck.HealthCheck,
) {
	metricsFactory := baseFactory.Namespace("collector", nil)

	spanBuilder, err := collector.NewSpanHandlerBuilder(
		cOpts,
		spanWriter,
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
	)
	if err != nil {
		logger.Fatal("Unable to set up builder", zap.Error(err))
	}

	zipkinSpansHandler, jaegerBatchesHandler, grpcHandler := spanBuilder.BuildHandlers()

	{
		ch, err := tchannel.NewChannel("jaeger-collector", &tchannel.ChannelOptions{})
		if err != nil {
			logger.Fatal("Unable to create new TChannel", zap.Error(err))
		}
		server := thrift.NewServer(ch)
		server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
		server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))
		server.Register(sc.NewTChanSamplingManagerServer(sampling.NewHandler(strategyStore)))
		portStr := ":" + strconv.Itoa(cOpts.CollectorPort)
		listener, err := net.Listen("tcp", portStr)
		if err != nil {
			logger.Fatal("Unable to start listening on channel", zap.Error(err))
		}
		logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", cOpts.CollectorPort))
		ch.Serve(listener)
	}

	{
		grpcPortStr := ":" + strconv.Itoa(cOpts.CollectorGRPCPort)
		lis, err := net.Listen("tcp", grpcPortStr)
		if err != nil {
			logger.Fatal("Failed to listen on gRPC port", zap.Error(err))
		}

		log := grpclog.NewLoggerV2(os.Stdout, os.Stderr, os.Stderr)
		grpclog.SetLoggerV2(log)

		grpcSrv := grpc.NewServer()
		api_v2.RegisterCollectorServiceServer(grpcSrv, grpcHandler)
		api_v2.RegisterSamplingManagerServer(grpcSrv, sampling.NewGRPCHandler(strategyStore))
		logger.Info("Starting Jaeger Collector gRPC server", zap.Int("grpc-port", cOpts.CollectorGRPCPort))
		go func() {
			if err := grpcSrv.Serve(lis); err != nil {
				logger.Fatal("Could not launch gRPC service", zap.Error(err))
			}
			hc.Set(healthcheck.Unavailable)
		}()
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
	hc *healthcheck.HealthCheck,
) {
	tracer, closer, err := jaegerClientConfig.Configuration{
		Sampler: &jaegerClientConfig.SamplerConfig{
			Type:  "const",
			Param: 1.0,
		},
		RPCMetrics: true,
	}.New(
		"jaeger-query",
		jaegerClientConfig.Metrics(baseFactory.Namespace("client", nil)),
		jaegerClientConfig.Logger(jaegerClientZapLog.NewLogger(logger)),
	)
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	opentracing.SetGlobalTracer(tracer)

	spanReader = storageMetrics.NewReadMetricsDecorator(spanReader, baseFactory.Namespace("query", nil))

	apiHandler := queryApp.NewAPIHandler(
		spanReader,
		depReader,
		queryApp.HandlerOptions.Logger(logger),
		queryApp.HandlerOptions.Tracer(tracer))

	r := mux.NewRouter()
	if qOpts.BasePath != "/" {
		r = r.PathPrefix(qOpts.BasePath).Subrouter()
	}
	apiHandler.RegisterRoutes(r)
	queryApp.RegisterStaticHandler(r, logger, qOpts)

	if h := metricsBuilder.Handler(); h != nil {
		logger.Info("Registering metrics handler with jaeger-query HTTP server", zap.String("route", metricsBuilder.HTTPRoute))
		r.Handle(metricsBuilder.HTTPRoute, h)
	}

	portStr := ":" + strconv.Itoa(qOpts.Port)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	logger.Info("Starting jaeger-query HTTP server", zap.Int("port", qOpts.Port))
	go func() {
		defer closer.Close()
		if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
			logger.Fatal("Could not launch jaeger-query service", zap.Error(err))
		}
		hc.Set(healthcheck.Unavailable)
	}()
}

func initializeStrategyStore(
	samplingStrategyStoreFactory *ss.Factory,
	v *viper.Viper,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) strategystore.StrategyStore {
	samplingStrategyStoreFactory.InitFromViper(v)
	if err := samplingStrategyStoreFactory.Initialize(metricsFactory, logger); err != nil {
		logger.Fatal("Failed to init sampling strategy store factory", zap.Error(err))
	}
	strategyStore, err := samplingStrategyStoreFactory.CreateStrategyStore()
	if err != nil {
		logger.Fatal("Failed to create sampling strategy store", zap.Error(err))
	}
	return strategyStore
}
