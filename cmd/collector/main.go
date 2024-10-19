// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/internal/env"
	cmdFlags "github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/printconfig"
	"github.com/jaegertracing/jaeger/cmd/internal/status"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/version"
	ss "github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
)

const serviceName = "jaeger-collector"

func main() {
	svc := cmdFlags.NewService(ports.CollectorAdminHTTP)

	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}
	samplingStrategyFactoryConfig, err := ss.FactoryConfigFromEnv()
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy store factory config: %v", err)
	}
	samplingStrategyFactory, err := ss.NewFactory(*samplingStrategyFactoryConfig)
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy store factory: %v", err)
	}

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-collector",
		Short: "Jaeger collector receives and stores traces",
		Long:  `Jaeger collector receives traces and runs them through a processing pipeline.`,
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "collector"})
			version.NewInfoMetrics(metricsFactory)

			storageFactory.InitFromViper(v, logger)
			if err := storageFactory.Initialize(baseFactory, logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}
			spanWriter, err := storageFactory.CreateSpanWriter()
			if err != nil {
				logger.Fatal("Failed to create span writer", zap.Error(err))
			}

			ssFactory, err := storageFactory.CreateSamplingStoreFactory()
			if err != nil {
				logger.Fatal("Failed to create sampling strategy factory", zap.Error(err))
			}

			samplingStrategyFactory.InitFromViper(v, logger)
			if err := samplingStrategyFactory.Initialize(metricsFactory, ssFactory, logger); err != nil {
				logger.Fatal("Failed to init sampling strategy factory", zap.Error(err))
			}
			samplingProvider, samplingAggregator, err := samplingStrategyFactory.CreateStrategyProvider()
			if err != nil {
				logger.Fatal("Failed to create sampling strategy provider", zap.Error(err))
			}
			collectorOpts, err := new(flags.CollectorOptions).InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to initialize collector", zap.Error(err))
			}
			tm := tenancy.NewManager(&collectorOpts.GRPC.Tenancy)

			collector := app.New(&app.CollectorParams{
				ServiceName:        serviceName,
				Logger:             logger,
				MetricsFactory:     metricsFactory,
				SpanWriter:         spanWriter,
				SamplingProvider:   samplingProvider,
				SamplingAggregator: samplingAggregator,
				HealthCheck:        svc.HC(),
				TenancyMgr:         tm,
			})
			// Start all Collector services
			if err := collector.Start(collectorOpts); err != nil {
				logger.Fatal("Failed to start collector", zap.Error(err))
			}
			// Wait for shutdown
			svc.RunAndThen(func() {
				if err := collector.Close(); err != nil {
					logger.Error("failed to cleanly close the collector", zap.Error(err))
				}
				if closer, ok := spanWriter.(io.Closer); ok {
					err := closer.Close()
					if err != nil {
						logger.Error("failed to close span writer", zap.Error(err))
					}
				}
				if err := storageFactory.Close(); err != nil {
					logger.Error("Failed to close storage factory", zap.Error(err))
				}
				if err := samplingStrategyFactory.Close(); err != nil {
					logger.Error("Failed to close sampling strategy store factory", zap.Error(err))
				}
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.CollectorAdminHTTP))
	command.AddCommand(printconfig.Command(v))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		flags.AddFlags,
		storageFactory.AddPipelineFlags,
		samplingStrategyFactory.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
