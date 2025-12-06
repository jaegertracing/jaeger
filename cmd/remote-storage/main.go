// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel/metric/noop"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/internal/env"
	"github.com/jaegertracing/jaeger/cmd/internal/featuregate"
	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/printconfig"
	"github.com/jaegertracing/jaeger/cmd/internal/status"
	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	storage "github.com/jaegertracing/jaeger/internal/storage/v1/factory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/version"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	serviceName     = "jaeger-remote-storage"
	configFlagName  = "config"
	configFlagShort = "c"
)

func main() {
	svc := flags.NewService(ports.RemoteStorageAdminHTTP)

	v := viper.New()
	command := &cobra.Command{
		Use:   serviceName,
		Short: serviceName + " allows sharing single-node storage implementations like memstore or Badger.",
		Long:  serviceName + ` allows sharing single-node storage implementations like memstore or Badger. It implements Jaeger Remote Storage gRPC API.`,
		RunE: func(cmd *cobra.Command, _ /* args */ []string) error {
			// Check if config file is provided
			configFile, _ := cmd.Flags().GetString(configFlagName)

			if configFile != "" {
				// New YAML configuration path
				return runWithYAMLConfig(cmd, v, svc, configFile)
			}

			// Legacy CLI flags path (with deprecation warning)
			return runWithCLIFlags(cmd, v, svc)
		},
	}

	// Add config file flag
	command.Flags().StringP(configFlagName, configFlagShort, "", "Path to YAML configuration file (recommended)")

	// Legacy flags (deprecated)
	v1ConfigureFlags(v, command, svc)

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.RemoteStorageAdminHTTP))
	command.AddCommand(printconfig.Command(v))
	command.AddCommand(featuregate.Command())

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// v1ConfigureFlags configures legacy CLI flags (deprecated).
func v1ConfigureFlags(v *viper.Viper, command *cobra.Command, svc *flags.Service) {
	storageFactory, err := storage.NewFactory(storage.ConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		storageFactory.AddFlags,
		app.AddFlags,
	)
}

// runWithYAMLConfig runs the service with YAML configuration.
func runWithYAMLConfig(_ *cobra.Command, v *viper.Viper, svc *flags.Service, configFile string) error {
	// Load configuration from YAML file
	v.SetConfigFile(configFile)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := svc.Start(v); err != nil {
		return err
	}

	logger := svc.Logger
	logger.Info("Loading configuration from YAML file", zap.String("file", configFile))

	baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
	metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "remote-storage"})
	version.NewInfoMetrics(metricsFactory)

	// Load configuration
	cfg, err := app.LoadConfigFromViper(v)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	baseTelset := telemetry.Settings{
		Logger:        svc.Logger,
		Metrics:       baseFactory,
		ReportStatus:  telemetry.HCAdapter(svc.HC()),
		MeterProvider: noop.NewMeterProvider(),
	}

	serverOpts := cfg.GetServerOptions()
	tm := tenancy.NewManager(&serverOpts.Tenancy)
	telset := baseTelset
	telset.Metrics = metricsFactory

	// Get the storage name (first backend configured)
	storageName := cfg.GetStorageName()
	if storageName == "" {
		logger.Fatal("No storage backend configured")
	}

	// Create storage factory from configuration
	traceFactory, depFactory, err := app.CreateStorageFactory(
		context.Background(),
		storageName,
		&cfg.Storage,
		telset,
	)
	if err != nil {
		logger.Fatal("Failed to create storage factory", zap.Error(err))
	}

	// Create and start server
	server, err := app.NewServer(
		context.Background(),
		serverOpts,
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

	svc.RunAndThen(func() {
		server.Close()
		if closer, ok := traceFactory.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				logger.Error("Failed to close storage factory", zap.Error(err))
			}
		}
	})

	return nil
}

// runWithCLIFlags runs the service with legacy CLI flags (deprecated).
func runWithCLIFlags(_ *cobra.Command, v *viper.Viper, svc *flags.Service) error {
	// Print deprecation warning
	fmt.Fprintln(os.Stderr, "WARNING: CLI flags for storage configuration are deprecated and will be removed in a future release.")
	fmt.Fprintln(os.Stderr, "Please migrate to YAML configuration file using the --config flag.")
	fmt.Fprintln(os.Stderr, "See config.yaml for an example configuration.")
	fmt.Fprintln(os.Stderr, "")

	// Set default storage type if not specified
	if os.Getenv(storage.SpanStorageTypeEnvVar) == "" {
		os.Setenv(storage.SpanStorageTypeEnvVar, "memory")
	}

	storageFactory, err := storage.NewFactory(storage.ConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		return fmt.Errorf("cannot initialize storage factory: %w", err)
	}

	if err := svc.Start(v); err != nil {
		return err
	}

	logger := svc.Logger
	baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
	metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "remote-storage"})
	version.NewInfoMetrics(metricsFactory)

	opts, err := new(app.Options).InitFromViper(v)
	if err != nil {
		logger.Fatal("Failed to parse options", zap.Error(err))
	}

	baseTelset := telemetry.Settings{
		Logger:        svc.Logger,
		Metrics:       baseFactory,
		ReportStatus:  telemetry.HCAdapter(svc.HC()),
		MeterProvider: noop.NewMeterProvider(),
	}

	storageFactory.InitFromViper(v, logger)
	if err := storageFactory.Initialize(baseTelset.Metrics, baseTelset.Logger); err != nil {
		logger.Fatal("Failed to init storage factory", zap.Error(err))
	}

	tm := tenancy.NewManager(&opts.Tenancy)
	telset := baseTelset
	telset.Metrics = metricsFactory

	traceFactory := v1adapter.NewFactory(storageFactory)
	depFactory := traceFactory.(depstore.Factory)
	server, err := app.NewServer(context.Background(), opts, v1adapter.NewFactory(storageFactory), depFactory, tm, telset)
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
	})

	return nil
}
