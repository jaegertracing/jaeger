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
	"log"
	"os"

	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	"github.com/uber/jaeger-lib/metrics"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/cmd/status"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
	metricsPlugin "github.com/jaegertracing/jaeger/plugin/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

func main() {
	svc := flags.NewService(ports.QueryAdminHTTP)

	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}

	fc := metricsPlugin.FactoryConfigFromEnv()
	metricsReaderFactory, err := metricsPlugin.NewFactory(fc)
	if err != nil {
		log.Fatalf("Cannot initialize metrics factory: %v", err)
	}

	v := viper.New()
	var command = &cobra.Command{
		Use:   "jaeger-query",
		Short: "Jaeger query service provides a Web UI and an API for accessing trace data.",
		Long:  `Jaeger query service provides a Web UI and an API for accessing trace data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "query"})
			version.NewInfoMetrics(metricsFactory)

			traceCfg := &jaegerClientConfig.Configuration{
				ServiceName: "jaeger-query",
				Sampler: &jaegerClientConfig.SamplerConfig{
					Type:  "const",
					Param: 1.0,
				},
				RPCMetrics: true,
			}
			traceCfg, err = traceCfg.FromEnv()
			if err != nil {
				logger.Fatal("Failed to read tracer configuration", zap.Error(err))
			}
			tracer, closer, err := traceCfg.NewTracer(
				jaegerClientConfig.Metrics(svc.MetricsFactory),
				jaegerClientConfig.Logger(jaegerClientZapLog.NewLogger(logger)),
			)
			if err != nil {
				logger.Fatal("Failed to initialize tracer", zap.Error(err))
			}
			defer closer.Close()
			opentracing.SetGlobalTracer(tracer)
			queryOpts, err := new(app.QueryOptions).InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to configure query service", zap.Error(err))
			}
			// TODO: Need to figure out set enable/disable propagation on storage plugins.
			v.Set(bearertoken.StoragePropagationKey, queryOpts.BearerTokenPropagation)
			storageFactory.InitFromViper(v, logger)
			if err := storageFactory.Initialize(baseFactory, logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}
			spanReader, err := storageFactory.CreateSpanReader()
			if err != nil {
				logger.Fatal("Failed to create span reader", zap.Error(err))
			}
			spanReader = storageMetrics.NewReadMetricsDecorator(spanReader, metricsFactory)
			dependencyReader, err := storageFactory.CreateDependencyReader()
			if err != nil {
				logger.Fatal("Failed to create dependency reader", zap.Error(err))
			}

			metricsQueryService, err := createMetricsQueryService(metricsReaderFactory, v, logger)
			if err != nil {
				logger.Fatal("Failed to create metrics query service", zap.Error(err))
			}
			queryServiceOptions := queryOpts.BuildQueryServiceOptions(storageFactory, logger)
			queryService := querysvc.NewQueryService(
				spanReader,
				dependencyReader,
				*queryServiceOptions)
			server, err := app.NewServer(svc.Logger, queryService, metricsQueryService, queryOpts, tracer)
			if err != nil {
				logger.Fatal("Failed to create server", zap.Error(err))
			}

			go func() {
				for s := range server.HealthCheckStatus() {
					svc.SetHealthCheckStatus(s)
				}
			}()

			if err := server.Start(); err != nil {
				logger.Fatal("Could not start servers", zap.Error(err))
			}

			svc.RunAndThen(func() {
				server.Close()
				if err := storageFactory.Close(); err != nil {
					logger.Error("Failed to close storage factory", zap.Error(err))
				}
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.QueryAdminHTTP))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		storageFactory.AddFlags,
		app.AddFlags,
		metricsReaderFactory.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func createMetricsQueryService(factory *metricsPlugin.Factory, v *viper.Viper, logger *zap.Logger) (querysvc.MetricsQueryService, error) {
	if err := factory.Initialize(logger); err != nil {
		return nil, fmt.Errorf("failed to init metrics reader factory: %w", err)
	}

	// Ensure default parameter values are loaded correctly.
	factory.InitFromViper(v, logger)
	return factory.CreateMetricsReader()
}
