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
	"io"
	"log"
	"os"

	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	"github.com/uber/jaeger-lib/metrics"
	jexpvar "github.com/uber/jaeger-lib/metrics/expvar"
	"github.com/uber/jaeger-lib/metrics/fork"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	agentRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	agentGrpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/all-in-one/setupcontext"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/cmd/status"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
	ss "github.com/jaegertracing/jaeger/plugin/sampling/strategystore"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
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
			metricsFactory := fork.New("internal",
				jexpvar.NewFactory(10), // backend for internal opts
				rootMetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"}))

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
			if err := strategyStoreFactory.Initialize(metricsFactory, logger); err != nil {
				logger.Fatal("Failed to init sampling strategy store factory", zap.Error(err))
			}
			strategyStore, err := strategyStoreFactory.CreateStrategyStore()
			if err != nil {
				logger.Fatal("Failed to create sampling strategy store", zap.Error(err))
			}

			aOpts := new(agentApp.Builder).InitFromViper(v)
			repOpts := new(agentRep.Options).InitFromViper(v, logger)
			grpcBuilder := agentGrpcRep.NewConnBuilder().InitFromViper(v)
			cOpts := new(collectorApp.CollectorOptions).InitFromViper(v)
			qOpts := new(queryApp.QueryOptions).InitFromViper(v, logger)

			// collector
			c := collectorApp.New(&collectorApp.CollectorParams{
				ServiceName:    "jaeger-collector",
				Logger:         logger,
				MetricsFactory: metricsFactory,
				SpanWriter:     spanWriter,
				StrategyStore:  strategyStore,
				HealthCheck:    svc.HC(),
			})
			if err := c.Start(cOpts); err != nil {
				log.Fatal(err)
			}

			// agent
			// if the agent reporter grpc host:port was not explicitly set then use whatever the collector is listening on
			if len(grpcBuilder.CollectorHostPorts) == 0 {
				grpcBuilder.CollectorHostPorts = append(grpcBuilder.CollectorHostPorts, cOpts.CollectorGRPCHostPort)
			}
			agentMetricsFactory := metricsFactory.Namespace(metrics.NSOptions{Name: "agent", Tags: nil})
			builders := map[agentRep.Type]agentApp.CollectorProxyBuilder{
				agentRep.GRPC: agentApp.GRPCCollectorProxyBuilder(grpcBuilder),
			}
			cp, err := agentApp.CreateCollectorProxy(agentApp.ProxyBuilderOptions{
				Options: *repOpts,
				Logger:  logger,
				Metrics: agentMetricsFactory,
			}, builders)
			if err != nil {
				logger.Fatal("Could not create collector proxy", zap.Error(err))
			}
			agent := startAgent(cp, aOpts, logger, metricsFactory)

			// query
			querySrv := startQuery(
				svc, qOpts, qOpts.BuildQueryServiceOptions(storageFactory, logger),
				spanReader, dependencyReader,
				rootMetricsFactory, metricsFactory,
			)

			svc.RunAndThen(func() {
				agent.Stop()
				_ = cp.Close()
				_ = c.Close()
				_ = querySrv.Close()
				if closer, ok := spanWriter.(io.Closer); ok {
					if err := closer.Close(); err != nil {
						logger.Error("Failed to close span writer", zap.Error(err))
					}
				}
				if err := storageFactory.Close(); err != nil {
					logger.Error("Failed to close storage factory", zap.Error(err))
				}
				_ = tracerCloser.Close()
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.CollectorAdminHTTP))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		storageFactory.AddFlags,
		agentApp.AddFlags,
		agentRep.AddFlags,
		agentGrpcRep.AddFlags,
		collectorApp.AddFlags,
		queryApp.AddFlags,
		strategyStoreFactory.AddFlags,
	)

	if err := command.Execute(); err != nil {
		log.Fatal(err)
	}
}

func startAgent(
	cp agentApp.CollectorProxy,
	b *agentApp.Builder,
	logger *zap.Logger,
	baseFactory metrics.Factory,
) *agentApp.Agent {

	agent, err := b.CreateAgent(cp, logger, baseFactory)
	if err != nil {
		logger.Fatal("Unable to initialize Jaeger Agent", zap.Error(err))
	}

	logger.Info("Starting agent")
	if err := agent.Run(); err != nil {
		logger.Fatal("Failed to run the agent", zap.Error(err))
	}

	return agent
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
	server, err := queryApp.NewServer(svc.Logger, qs, qOpts, opentracing.GlobalTracer())
	if err != nil {
		svc.Logger.Fatal("Could not start jaeger-query service", zap.Error(err))
	}
	go func() {
		for s := range server.HealthCheckStatus() {
			svc.SetHealthCheckStatus(s)
		}
	}()
	if err := server.Start(); err != nil {
		svc.Logger.Fatal("Could not start jaeger-query service", zap.Error(err))
	}
	return server
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
