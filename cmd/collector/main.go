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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	basicB "github.com/jaegertracing/jaeger/cmd/builder"
	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/builder"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	ss "github.com/jaegertracing/jaeger/plugin/sampling/strategystore"
	"github.com/jaegertracing/jaeger/plugin/storage"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	sc "github.com/jaegertracing/jaeger/thrift-gen/sampling"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const serviceName = "jaeger-collector"

func main() {
	var signalsChannel = make(chan os.Signal, 0)
	signal.Notify(signalsChannel, os.Interrupt, syscall.SIGTERM)

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Cannot initialize logger: %v", err)
	}
	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, logger))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}
	strategyStoreFactory, err := ss.NewFactory(ss.FactoryConfigFromEnv())
	if err != nil {
		log.Fatalf("Cannot initialize sampling strategy store factory: %v", err)
	}
	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-collector",
		Short: "Jaeger collector receives and processes traces from Jaeger agents and clients",
		Long:  `Jaeger collector receives traces from Jaeger agents and runs them through a processing pipeline.`,
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

			builderOpts := new(builder.CollectorOptions).InitFromViper(v)

			mBldr := new(pMetrics.Builder).InitFromViper(v)
			baseFactory, err := mBldr.CreateMetricsFactory("jaeger")
			if err != nil {
				logger.Fatal("Cannot create metrics factory.", zap.Error(err))
			}

			storageFactory.InitFromViper(v)
			if err := storageFactory.Initialize(baseFactory, logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}
			spanWriter, err := storageFactory.CreateSpanWriter()
			if err != nil {
				logger.Fatal("Failed to create span writer", zap.Error(err))
			}

			metricsFactory := baseFactory.Namespace("collector", nil)
			handlerBuilder, err := builder.NewSpanHandlerBuilder(
				builderOpts,
				spanWriter,
				basicB.Options.LoggerOption(logger),
				basicB.Options.MetricsFactoryOption(metricsFactory),
			)
			if err != nil {
				logger.Fatal("Unable to set up builder", zap.Error(err))
			}

			ch, err := tchannel.NewChannel(serviceName, &tchannel.ChannelOptions{})
			if err != nil {
				logger.Fatal("Unable to create new TChannel", zap.Error(err))
			}
			server := thrift.NewServer(ch)
			zipkinSpansHandler, jaegerBatchesHandler := handlerBuilder.BuildHandlers()
			server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
			server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))

			samplingHandler := initializeSamplingHandler(strategyStoreFactory, v, metricsFactory, logger)
			server.Register(sc.NewTChanSamplingManagerServer(samplingHandler))

			portStr := ":" + strconv.Itoa(builderOpts.CollectorPort)
			listener, err := net.Listen("tcp", portStr)
			if err != nil {
				logger.Fatal("Unable to start listening on channel", zap.Error(err))
			}
			ch.Serve(listener)

			r := mux.NewRouter()
			apiHandler := app.NewAPIHandler(jaegerBatchesHandler)
			apiHandler.RegisterRoutes(r)
			if h := mBldr.Handler(); h != nil {
				logger.Info("Registering metrics handler with HTTP server", zap.String("route", mBldr.HTTPRoute))
				r.Handle(mBldr.HTTPRoute, h)
			}
			httpPortStr := ":" + strconv.Itoa(builderOpts.CollectorHTTPPort)
			recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

			go startZipkinHTTPAPI(logger, builderOpts.CollectorZipkinHTTPPort, zipkinSpansHandler, recoveryHandler)

			logger.Info("Starting Jaeger Collector HTTP server", zap.Int("http-port", builderOpts.CollectorHTTPPort))

			go func() {
				if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
					logger.Fatal("Could not launch service", zap.Error(err))
				}
				hc.Set(healthcheck.Unavailable)
			}()

			hc.Ready()
			select {
			case <-signalsChannel:
				if closer, ok := spanWriter.(io.Closer); ok {
					err := closer.Close()
					if err != nil {
						logger.Error("Failed to close span writer", zap.Error(err))
					}
				}

				logger.Info("Jaeger Collector is finishing")
			}
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())

	flags.SetDefaultHealthCheckPort(builder.CollectorDefaultHealthCheckHTTPPort)

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		builder.AddFlags,
		storageFactory.AddFlags,
		pMetrics.AddFlags,
		strategyStoreFactory.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func startZipkinHTTPAPI(
	logger *zap.Logger,
	zipkinPort int,
	zipkinSpansHandler app.ZipkinSpansHandler,
	recoveryHandler func(http.Handler) http.Handler,
) {
	if zipkinPort != 0 {
		zHandler := zipkin.NewAPIHandler(zipkinSpansHandler)
		r := mux.NewRouter()
		zHandler.RegisterRoutes(r)

		httpPortStr := ":" + strconv.Itoa(zipkinPort)
		logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", zipkinPort))

		if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
			logger.Fatal("Could not launch service", zap.Error(err))
		}
	}
}

func initializeSamplingHandler(
	samplingStrategyStoreFactory *ss.Factory,
	v *viper.Viper,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) sampling.Handler {
	samplingStrategyStoreFactory.InitFromViper(v)
	if err := samplingStrategyStoreFactory.Initialize(metricsFactory, logger); err != nil {
		logger.Fatal("Failed to init sampling strategy store factory", zap.Error(err))
	}
	strategyStore, err := samplingStrategyStoreFactory.CreateStrategyStore()
	if err != nil {
		logger.Fatal("Failed to create sampling strategy store", zap.Error(err))
	}
	return sampling.NewHandler(strategyStore)
}
