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
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	agentRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	agentGrpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/all-in-one/setupcontext"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	collectorFlags "github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/cmd/status"
	"github.com/jaegertracing/jaeger/internal/metrics/expvar"
	"github.com/jaegertracing/jaeger/internal/metrics/fork"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/version"
	metricsPlugin "github.com/jaegertracing/jaeger/plugin/metrics"
	ss "github.com/jaegertracing/jaeger/plugin/sampling/strategystore"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	metricsstoreMetrics "github.com/jaegertracing/jaeger/storage/metricsstore/metrics"
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
	strategyStoreFactoryConfig, err := ss.FactoryConfigFromEnv(os.Stderr)
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy store factory config: %v", err)
	}
	strategyStoreFactory, err := ss.NewFactory(*strategyStoreFactoryConfig)
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy store factory: %v", err)
	}

	fc := metricsPlugin.FactoryConfigFromEnv()
	metricsReaderFactory, err := metricsPlugin.NewFactory(fc)
	if err != nil {
		log.Fatalf("Cannot initialize metrics store factory: %v", err)
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
			logger := svc.Logger // shortcut
			metricsFactory := fork.New("internal",
				expvar.NewFactory(10), // backend for internal opts
				svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"}))
			version.NewInfoMetrics(metricsFactory)

			tracer, err := jtracer.New("jaeger-all-in-one")
			if err != nil {
				logger.Fatal("Failed to initialize tracer", zap.Error(err))
			}
			otel.SetTracerProvider(tracer.OTEL)

			storageFactory.InitFromViper(v, logger)
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

			metricsQueryService, err := createMetricsQueryService(metricsReaderFactory, v, logger, metricsFactory)
			if err != nil {
				logger.Fatal("Failed to create metrics reader", zap.Error(err))
			}

			ssFactory, err := storageFactory.CreateSamplingStoreFactory()
			if err != nil {
				logger.Fatal("Failed to create sampling store factory", zap.Error(err))
			}

			strategyStoreFactory.InitFromViper(v, logger)
			if err := strategyStoreFactory.Initialize(metricsFactory, ssFactory, logger); err != nil {
				logger.Fatal("Failed to init sampling strategy store factory", zap.Error(err))
			}
			strategyStore, aggregator, err := strategyStoreFactory.CreateStrategyStore()
			if err != nil {
				logger.Fatal("Failed to create sampling strategy store", zap.Error(err))
			}

			aOpts := new(agentApp.Builder).InitFromViper(v)
			repOpts := new(agentRep.Options).InitFromViper(v, logger)
			grpcBuilder, err := agentGrpcRep.NewConnBuilder().InitFromViper(v)
			if err != nil {
				logger.Fatal("Failed to configure connection for grpc", zap.Error(err))
			}
			cOpts, err := new(collectorFlags.CollectorOptions).InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to initialize collector", zap.Error(err))
			}
			qOpts, err := new(queryApp.QueryOptions).InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to configure query service", zap.Error(err))
			}

			tm := tenancy.NewManager(&cOpts.GRPC.Tenancy)

			// collector
			c := collectorApp.New(&collectorApp.CollectorParams{
				ServiceName:    "jaeger-collector",
				Logger:         logger,
				MetricsFactory: metricsFactory,
				SpanWriter:     spanWriter,
				StrategyStore:  strategyStore,
				Aggregator:     aggregator,
				HealthCheck:    svc.HC(),
				TenancyMgr:     tm,
			})
			if err := c.Start(cOpts); err != nil {
				log.Fatal(err)
			}

			// agent
			// if the agent reporter grpc host:port was not explicitly set then use whatever the collector is listening on
			if len(grpcBuilder.CollectorHostPorts) == 0 {
				grpcBuilder.CollectorHostPorts = append(grpcBuilder.CollectorHostPorts, cOpts.GRPC.HostPort)
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
				spanReader, dependencyReader, metricsQueryService,
				metricsFactory, tm, tracer,
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
				if err := tracer.Close(context.Background()); err != nil {
					logger.Error("Error shutting down tracer provider", zap.Error(err))
				}
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
		storageFactory.AddPipelineFlags,
		agentApp.AddFlags,
		agentRep.AddFlags,
		agentGrpcRep.AddFlags,
		collectorFlags.AddFlags,
		queryApp.AddFlags,
		strategyStoreFactory.AddFlags,
		metricsReaderFactory.AddFlags,
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
	metricsQueryService querysvc.MetricsQueryService,
	baseFactory metrics.Factory,
	tm *tenancy.Manager,
	jt *jtracer.JTracer,
) *queryApp.Server {
	spanReader = storageMetrics.NewReadMetricsDecorator(spanReader, baseFactory.Namespace(metrics.NSOptions{Name: "query"}))
	qs := querysvc.NewQueryService(spanReader, depReader, *queryOpts)
	server, err := queryApp.NewServer(svc.Logger, qs, metricsQueryService, qOpts, tm, jt)
	if err != nil {
		svc.Logger.Fatal("Could not create jaeger-query", zap.Error(err))
	}
	go func() {
		for s := range server.HealthCheckStatus() {
			svc.SetHealthCheckStatus(s)
		}
	}()
	if err := server.Start(); err != nil {
		svc.Logger.Fatal("Could not start jaeger-query", zap.Error(err))
	}

	return server
}

func createMetricsQueryService(
	metricsReaderFactory *metricsPlugin.Factory,
	v *viper.Viper,
	logger *zap.Logger,
	metricsReaderMetricsFactory metrics.Factory,
) (querysvc.MetricsQueryService, error) {
	if err := metricsReaderFactory.Initialize(logger); err != nil {
		return nil, fmt.Errorf("failed to init metrics reader factory: %w", err)
	}

	// Ensure default parameter values are loaded correctly.
	metricsReaderFactory.InitFromViper(v, logger)
	reader, err := metricsReaderFactory.CreateMetricsReader()
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics reader: %w", err)
	}

	// Decorate the metrics reader with metrics instrumentation.
	return metricsstoreMetrics.NewReadMetricsDecorator(reader, metricsReaderMetricsFactory), nil
}
