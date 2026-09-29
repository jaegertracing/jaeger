// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel/metric/noop"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/internal/featuregate"
	"github.com/jaegertracing/jaeger/cmd/internal/printconfig"
	"github.com/jaegertracing/jaeger/cmd/internal/status"
	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/version"
	"github.com/jaegertracing/jaeger/ports"
)

const serviceName = "jaeger-remote-storage"

// loadConfig reads the configuration file named by --config-file, or returns the default
// configuration when none was given.
func loadConfig(ctx context.Context, v *viper.Viper) (*app.Config, error) {
	path := app.ConfigFile(v)
	if path == "" {
		return app.DefaultConfig(), nil
	}
	return app.LoadConfigFile(ctx, path)
}

func main() {
	svc := app.NewService()

	v := viper.New()
	command := &cobra.Command{
		Use:   serviceName,
		Short: serviceName + " allows sharing single-node storage implementations like memstore or Badger.",
		Long:  serviceName + ` allows sharing single-node storage implementations like memstore or Badger. It implements Jaeger Remote Storage gRPC API.`,
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			cfg, err := loadConfig(context.Background(), v)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}
			if err := svc.Start(cfg.Service); err != nil {
				return err
			}
			logger := svc.Logger
			if app.ConfigFile(v) == "" {
				logger.Info("No configuration file provided, using default configuration (memory storage on :17271)")
			}
			baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "remote-storage"})
			version.NewInfoMetrics(metricsFactory)

			baseTelset := telemetry.Settings{
				Logger:        svc.Logger,
				Metrics:       baseFactory,
				Host:          svc.Admin.Host(),
				MeterProvider: noop.NewMeterProvider(),
			}

			tm := tenancy.NewManager(&cfg.Tenancy)
			telset := baseTelset
			telset.Metrics = metricsFactory

			// Get the storage name (first backend configured)
			storageName := cfg.GetStorageName()
			if storageName == "" {
				logger.Fatal("No storage backend configured")
			}

			// Get the backend configuration
			backend, ok := cfg.Storage.TraceBackends[storageName]
			if !ok {
				logger.Fatal("Storage backend not found", zap.String("name", storageName))
			}

			// Create storage factory from configuration (no auth resolver for remote-storage)
			traceFactory, err := storageconfig.CreateTraceStorageFactory(
				context.Background(),
				storageName,
				backend,
				telset,
				nil, // no auth resolver for remote-storage
			)
			if err != nil {
				logger.Fatal("Failed to create storage factory", zap.Error(err))
			}

			depFactory, ok := traceFactory.(depstore.Factory)
			if !ok {
				logger.Fatal("Storage does not implement dependency store", zap.String("name", storageName))
			}

			// Create and start server
			server, err := app.NewServer(
				context.Background(),
				cfg.GRPC,
				traceFactory,
				depFactory,
				tm,
				telset,
			)
			if err != nil {
				logger.Fatal("Failed to create server", zap.Error(err))
			}

			if err := server.Start(context.Background()); err != nil {
				logger.Fatal("Could not start servers", zap.Error(err))
			}

			return svc.RunAndThen(func() {
				server.Close()
				if closer, ok := traceFactory.(io.Closer); ok {
					if err := closer.Close(); err != nil {
						logger.Error("Failed to close storage factory", zap.Error(err))
					}
				}
			})
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.RemoteStorageAdminHTTP))
	command.AddCommand(printconfig.Command(v))
	command.AddCommand(featuregate.Command())

	// Add only basic flags (not storage flags)
	config.AddFlags(
		v,
		command,
		svc.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
