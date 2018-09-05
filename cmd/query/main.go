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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gorilla/handlers"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/plugin/storage"
	istorage "github.com/jaegertracing/jaeger/storage"
)

func main() {
	var serverChannel = make(chan os.Signal, 0)
	signal.Notify(serverChannel, os.Interrupt, syscall.SIGTERM)

	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}

	v := viper.New()

	var command = &cobra.Command{
		Use:   "jaeger-query",
		Short: "Jaeger query service provides a Web UI and an API for accessing trace data.",
		Long:  `Jaeger query service provides a Web UI and an API for accessing trace data.`,
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

			queryOpts := new(app.QueryOptions).InitFromViper(v)

			mBldr := new(pMetrics.Builder).InitFromViper(v)
			baseFactory, err := mBldr.CreateMetricsFactory("jaeger")
			if err != nil {
				logger.Fatal("Cannot create metrics factory.", zap.Error(err))
			}

			tracer, closer, err := jaegerClientConfig.Configuration{
				Sampler: &jaegerClientConfig.SamplerConfig{
					Type:  "probabilistic",
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
			defer closer.Close()
			opentracing.SetGlobalTracer(tracer)

			storageFactory.InitFromViper(v)
			if err := storageFactory.Initialize(baseFactory, logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}
			spanReader, err := storageFactory.CreateSpanReader()
			if err != nil {
				logger.Fatal("Failed to create span reader", zap.Error(err))
			}
			dependencyReader, err := storageFactory.CreateDependencyReader()
			if err != nil {
				logger.Fatal("Failed to create dependency reader", zap.Error(err))
			}

			apiHandlerOptions := []app.HandlerOption{
				app.HandlerOptions.Logger(logger),
				app.HandlerOptions.Tracer(tracer),
			}
			apiHandlerOptions = append(apiHandlerOptions, archiveOptions(storageFactory, logger)...)
			apiHandler := app.NewAPIHandler(
				spanReader,
				dependencyReader,
				apiHandlerOptions...)
			r := app.NewRouter()
			if queryOpts.BasePath != "/" {
				r = r.PathPrefix(queryOpts.BasePath).Subrouter()
			}
			apiHandler.RegisterRoutes(r)
			app.RegisterStaticHandler(r, logger, queryOpts)

			if h := mBldr.Handler(); h != nil {
				logger.Info("Registering metrics handler with HTTP server", zap.String("route", mBldr.HTTPRoute))
				r.Handle(mBldr.HTTPRoute, h)
			}

			portStr := ":" + strconv.Itoa(queryOpts.Port)
			compressHandler := handlers.CompressHandler(r)
			recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

			go func() {
				logger.Info("Starting jaeger-query HTTP server", zap.Int("port", queryOpts.Port))
				if err := http.ListenAndServe(portStr, recoveryHandler(compressHandler)); err != nil {
					logger.Fatal("Could not launch service", zap.Error(err))
				}
				hc.Set(healthcheck.Unavailable)
			}()

			hc.Ready()

			select {
			case <-serverChannel:
				logger.Info("Jaeger Query is finishing")
			}
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())

	flags.SetDefaultHealthCheckPort(app.QueryDefaultHealthCheckHTTPPort)

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		storageFactory.AddFlags,
		pMetrics.AddFlags,
		app.AddFlags,
	)

	if error := command.Execute(); error != nil {
		fmt.Println(error.Error())
		os.Exit(1)
	}
}

func archiveOptions(storageFactory istorage.Factory, logger *zap.Logger) []app.HandlerOption {
	archiveFactory, ok := storageFactory.(istorage.ArchiveFactory)
	if !ok {
		logger.Info("Archive storage not supported by the factory")
		return nil
	}
	reader, err := archiveFactory.CreateArchiveSpanReader()
	if err == istorage.ErrArchiveStorageNotConfigured || err == istorage.ErrArchiveStorageNotSupported {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return nil
	}
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return nil
	}
	writer, err := archiveFactory.CreateArchiveSpanWriter()
	if err == istorage.ErrArchiveStorageNotConfigured || err == istorage.ErrArchiveStorageNotSupported {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return nil
	}
	if err != nil {
		logger.Error("Cannot init archive storage writer", zap.Error(err))
		return nil
	}
	return []app.HandlerOption{
		app.HandlerOptions.ArchiveSpanReader(reader),
		app.HandlerOptions.ArchiveSpanWriter(writer),
	}
}
