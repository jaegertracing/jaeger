// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	collectorFlags "github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/internal/env"
	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/printconfig"
	"github.com/jaegertracing/jaeger/cmd/internal/status"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	v2querysvc "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/jtracer"
	"github.com/jaegertracing/jaeger/internal/metrics"
	ss "github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/metafactory"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore"
	storage "github.com/jaegertracing/jaeger/internal/storage/v1/factory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/version"
	"github.com/jaegertracing/jaeger/ports"
)

// all-in-one/main is a standalone full-stack jaeger backend, backed by a memory store
func main() {
	flags.PrintV1EOL()

	svc := flags.NewService(ports.CollectorAdminHTTP)

	if os.Getenv(storage.SpanStorageTypeEnvVar) == "" {
		os.Setenv(storage.SpanStorageTypeEnvVar, "memory") // other storage types default to SpanStorage
	}
	storageFactory, err := storage.NewFactory(storage.ConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}
	samplingStrategyFactoryConfig, err := ss.FactoryConfigFromEnv()
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy factory config: %v", err)
	}
	samplingStrategyFactory, err := ss.NewFactory(*samplingStrategyFactoryConfig)
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy factory: %v", err)
	}

	fc := metricstore.FactoryConfigFromEnv()
	metricsReaderFactory, err := metricstore.NewFactory(fc)
	if err != nil {
		log.Fatalf("Cannot initialize metrics store factory: %v", err)
	}

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-all-in-one",
		Short: "Jaeger all-in-one distribution with collector and query in one process.",
		Long: `Jaeger all-in-one distribution with collector and query. Use with caution: this version
by default uses only in-memory database.`,
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			version.NewInfoMetrics(baseFactory)
			collectorMetricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "collector"})
			queryMetricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "query"})

			tracer, err := jtracer.New("jaeger-all-in-one")
			if err != nil {
				logger.Fatal("Failed to initialize tracer", zap.Error(err))
			}

			baseTelset := telemetry.Settings{
				Logger:         svc.Logger,
				TracerProvider: tracer.OTEL,
				Metrics:        baseFactory,
				MeterProvider:  noopmetric.NewMeterProvider(),
				ReportStatus:   telemetry.HCAdapter(svc.HC()),
			}

			storageFactory.InitFromViper(v, logger)
			if err := storageFactory.Initialize(baseTelset.Metrics, baseTelset.Logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}

			v2Factory := v1adapter.NewFactory(storageFactory)
			traceReader, err := v2Factory.CreateTraceReader()
			if err != nil {
				logger.Fatal("Failed to create span reader", zap.Error(err))
			}
			spanWriter, err := storageFactory.CreateSpanWriter()
			if err != nil {
				logger.Fatal("Failed to create span writer", zap.Error(err))
			}
			depstoreFactory, ok := v2Factory.(depstore.Factory)
			if !ok {
				logger.Fatal("Failed to create dependency reader", zap.Error(err))
			}
			dependencyReader, err := depstoreFactory.CreateDependencyReader()
			if err != nil {
				logger.Fatal("Failed to create dependency reader", zap.Error(err))
			}

			metricsQueryService, err := createMetricsQueryService(metricsReaderFactory, v, baseTelset)
			if err != nil {
				logger.Fatal("Failed to create metrics reader", zap.Error(err))
			}

			ssFactory, err := storageFactory.CreateSamplingStoreFactory()
			if err != nil {
				logger.Fatal("Failed to create sampling store factory", zap.Error(err))
			}

			samplingStrategyFactory.InitFromViper(v, logger)
			if err := samplingStrategyFactory.Initialize(collectorMetricsFactory, ssFactory, logger); err != nil {
				logger.Fatal("Failed to init sampling strategy factory", zap.Error(err))
			}
			samplingProvider, samplingAggregator, err := samplingStrategyFactory.CreateStrategyProvider()
			if err != nil {
				logger.Fatal("Failed to create sampling strategy provider", zap.Error(err))
			}

			cOpts, err := new(collectorFlags.CollectorOptions).InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to initialize collector", zap.Error(err))
			}
			defaultOpts := queryApp.DefaultQueryOptions()
			qOpts, err := defaultOpts.InitFromViper(v, logger)
			if err != nil {
				logger.Fatal("Failed to configure query service", zap.Error(err))
			}

			tm := tenancy.NewManager(&cOpts.Tenancy)

			// collector
			c := collectorApp.New(&collectorApp.CollectorParams{
				ServiceName:        "jaeger-collector",
				Logger:             logger,
				MetricsFactory:     collectorMetricsFactory,
				TraceWriter:        v1adapter.NewTraceWriter(spanWriter),
				SamplingProvider:   samplingProvider,
				SamplingAggregator: samplingAggregator,
				HealthCheck:        svc.HC(),
				TenancyMgr:         tm,
			})
			if err := c.Start(cOpts); err != nil {
				log.Fatal(err)
			}

			// query
			queryTelset := baseTelset // copy
			queryTelset.Metrics = queryMetricsFactory
			querySvcOpts, v2querySvcOpts := qOpts.BuildQueryServiceOptions(storageFactory.InitArchiveStorage, logger)
			querySrv := startQuery(
				svc, qOpts, querySvcOpts, v2querySvcOpts,
				traceReader, dependencyReader, metricsQueryService,
				tm, queryTelset,
			)

			svc.RunAndThen(func() {
				var errs []error
				errs = append(errs, c.Close())
				errs = append(errs, querySrv.Close())
				if closer, ok := spanWriter.(io.Closer); ok {
					errs = append(errs, closer.Close())
				}
				errs = append(errs, storageFactory.Close())
				errs = append(errs, tracer.Close(context.Background()))
				if err := errors.Join(errs...); err != nil {
					logger.Error("Failed to close services", zap.Error(err))
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
		storageFactory.AddPipelineFlags,
		collectorFlags.AddFlags,
		queryApp.AddFlags,
		samplingStrategyFactory.AddFlags,
		metricsReaderFactory.AddFlags,
	)

	if err := command.Execute(); err != nil {
		log.Fatal(err)
	}
}

func startQuery(
	svc *flags.Service,
	qOpts *queryApp.QueryOptions,
	queryOpts *querysvc.QueryServiceOptions,
	v2QueryOpts *v2querysvc.QueryServiceOptions,
	traceReader tracestore.Reader,
	depReader depstore.Reader,
	metricsQueryService querysvc.MetricsQueryService,
	tm *tenancy.Manager,
	telset telemetry.Settings,
) *queryApp.Server {
	qs := querysvc.NewQueryService(traceReader, depReader, *queryOpts)
	v2qs := v2querysvc.NewQueryService(traceReader, depReader, *v2QueryOpts)

	server, err := queryApp.NewServer(context.Background(), qs, v2qs, metricsQueryService, qOpts, tm, telset)
	if err != nil {
		svc.Logger.Fatal("Could not create jaeger-query", zap.Error(err))
	}
	if err := server.Start(context.Background()); err != nil {
		svc.Logger.Fatal("Could not start jaeger-query", zap.Error(err))
	}

	return server
}

func createMetricsQueryService(
	metricsReaderFactory *metricstore.Factory,
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
