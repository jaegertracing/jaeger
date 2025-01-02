// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/internal/env"
	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/printconfig"
	"github.com/jaegertracing/jaeger/cmd/internal/status"
	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	querysvcv2 "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/version"
	metricsPlugin "github.com/jaegertracing/jaeger/plugin/metricstore"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

func main() {
	flags.PrintV1EOL()
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
	command := &cobra.Command{
		Use:   "jaeger-query",
		Short: "Jaeger query service provides a Web UI and an API for accessing trace data.",
		Long:  `Jaeger query service provides a Web UI and an API for accessing trace data.`,
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "query"})
			version.NewInfoMetrics(metricsFactory)

			defaultOpts := app.DefaultQueryOptions()
			queryOpts, err := defaultOpts.InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to configure query service", zap.Error(err))
			}

			jt := jtracer.NoOp()
			if queryOpts.EnableTracing {
				jt, err = jtracer.New("jaeger-query")
				if err != nil {
					logger.Fatal("Failed to create tracer", zap.Error(err))
				}
			}

			baseTelset := telemetry.Settings{
				Logger:         logger,
				Metrics:        baseFactory,
				TracerProvider: jt.OTEL,
				ReportStatus:   telemetry.HCAdapter(svc.HC()),
			}

			// TODO: Need to figure out set enable/disable propagation on storage plugins.
			v.Set(bearertoken.StoragePropagationKey, queryOpts.BearerTokenPropagation)
			storageFactory.InitFromViper(v, logger)
			if err := storageFactory.Initialize(baseTelset.Metrics, baseTelset.Logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}

			v2Factory := v1adapter.NewFactory(storageFactory)
			traceReader, err := v2Factory.CreateTraceReader()
			if err != nil {
				logger.Fatal("Failed to create trace reader", zap.Error(err))
			}
			dependencyReader, err := v2Factory.CreateDependencyReader()
			if err != nil {
				logger.Fatal("Failed to create dependency reader", zap.Error(err))
			}

			metricsQueryService, err := createMetricsQueryService(metricsReaderFactory, v, baseTelset)
			if err != nil {
				logger.Fatal("Failed to create metrics query service", zap.Error(err))
			}
			queryServiceOptions := queryOpts.BuildQueryServiceOptions(storageFactory, logger)
			queryService := querysvc.NewQueryService(
				traceReader,
				dependencyReader,
				*queryServiceOptions)

			queryServiceOptionsV2 := queryOpts.BuildQueryServiceOptionsV2(storageFactory, logger)
			queryServiceV2 := querysvcv2.NewQueryService(
				traceReader,
				dependencyReader,
				*queryServiceOptionsV2)

			tm := tenancy.NewManager(&queryOpts.Tenancy)
			telset := baseTelset // copy
			telset.Metrics = metricsFactory
			server, err := app.NewServer(
				context.Background(),
				queryService,
				queryServiceV2,
				metricsQueryService,
				queryOpts,
				tm,
				telset,
			)
			if err != nil {
				logger.Fatal("Failed to create server", zap.Error(err))
			}

			if err := server.Start(context.Background()); err != nil {
				logger.Fatal("Could not start servers", zap.Error(err))
			}

			svc.RunAndThen(func() {
				server.Close()
				if err := storageFactory.Close(); err != nil {
					logger.Error("Failed to close storage factory", zap.Error(err))
				}
				if err = jt.Close(context.Background()); err != nil {
					logger.Fatal("Error shutting down tracer provider", zap.Error(err))
				}
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.QueryAdminHTTP))
	command.AddCommand(printconfig.Command(v))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		storageFactory.AddFlags,
		app.AddFlags,
		metricsReaderFactory.AddFlags,
		// add tenancy flags here to avoid panic caused by double registration in all-in-one
		tenancy.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func createMetricsQueryService(
	metricsReaderFactory *metricsPlugin.Factory,
	v *viper.Viper,
	telset telemetry.Settings,
) (querysvc.MetricsQueryService, error) {
	if err := metricsReaderFactory.Initialize(telset); err != nil {
		return nil, fmt.Errorf("failed to init metrics reader factory: %w", err)
	}

	// Ensure default parameter values are loaded correctly.
	metricsReaderFactory.InitFromViper(v, telset.Logger)
	reader, err := metricsReaderFactory.CreateMetricsReader()
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics reader: %w", err)
	}

	return reader, nil
}
