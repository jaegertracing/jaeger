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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tchannel "github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	tchReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	zs "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/relay/app"
	"github.com/jaegertracing/jaeger/cmd/relay/app/builder"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const serviceName = "jaeger-relay"

func main() {
	var signalsChannel = make(chan os.Signal)
	signal.Notify(signalsChannel, os.Interrupt, syscall.SIGTERM)

	v := viper.New()
	var command = &cobra.Command{
		Use:   "jaeger-relay",
		Short: "Jaeger relay is a utility program which relays span data.",
		Long:  `Jaeger relay is a utility program that can receive data over thrift/grpc and relay to thrift/grpc receiver.`,
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
				return err
			}

			builderOpts := new(builder.ReceiverOptions).InitFromViper(v)
			reporterOpts := new(builder.ReporterOptions).InitFromViper(v)

			mBldr := new(pMetrics.Builder).InitFromViper(v)
			baseFactory, err := mBldr.CreateMetricsFactory("jaeger")
			if err != nil {
				logger.Fatal("Cannot create metrics factory.", zap.Error(err))
				return err
			}

			metricsFactory := baseFactory.Namespace("relay", nil)
			ch, err := tchannel.NewChannel(serviceName, &tchannel.ChannelOptions{})
			if err != nil {
				logger.Fatal("Unable to create new TChannel", zap.Error(err))
				return err
			}
			server := thrift.NewServer(ch)
			tchrepbuilder := &tchReporter.Builder{
				CollectorHostPorts: reporterOpts.ReporterTChannelHostPorts,
				ConnCheckTimeout:   reporterOpts.ReporterTChannelDiscoveryConnCheckTimeout,
				DiscoveryMinPeers:  reporterOpts.ReporterTChannelDiscoveryMinPeers,
			}
			tchreporter, err := tchrepbuilder.CreateReporter(metricsFactory, logger)
			if err != nil {
				logger.Fatal("Cannot create tchannel reporter.", zap.Error(err))
				return err
			}
			spanBatchReporter := app.NewSpanBatchThriftReporter(
				tchreporter, metricsFactory, logger)
			jaegerBatchesHandler := collectorApp.NewJaegerSpanHandler(logger, spanBatchReporter)
			zSanitizer := zs.NewChainedSanitizer(
				zs.NewSpanDurationSanitizer(),
				zs.NewSpanStartTimeSanitizer(),
				zs.NewParentIDSanitizer(),
				zs.NewErrorTagSanitizer(),
			)
			zipkinSpansHandler := collectorApp.NewZipkinSpanHandler(logger, spanBatchReporter, zSanitizer)
			server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
			server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))

			portStr := ":" + strconv.Itoa(builderOpts.ReceiverJaegerTChannelPort)
			listener, err := net.Listen("tcp", portStr)
			if err != nil {
				logger.Fatal("Unable to start listening on channel", zap.Error(err))
			}
			ch.Serve(listener)

			r := mux.NewRouter()
			apiHandler := collectorApp.NewAPIHandler(jaegerBatchesHandler)
			apiHandler.RegisterRoutes(r)
			if h := mBldr.Handler(); h != nil {
				logger.Info("Registering metrics handler with HTTP server", zap.String("route", mBldr.HTTPRoute))
				r.Handle(mBldr.HTTPRoute, h)
			}
			httpPortStr := ":" + strconv.Itoa(builderOpts.ReceiverJaegerHTTPPort)
			recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

			go startZipkinHTTPAPI(logger, builderOpts.ReceiverZipkinHTTPPort, zipkinSpansHandler, recoveryHandler)

			logger.Info("Starting HTTP server", zap.Int("http-port", builderOpts.ReceiverJaegerHTTPPort))

			go func() {
				if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
					logger.Fatal("Could not launch service", zap.Error(err))
				}
				hc.Set(healthcheck.Unavailable)
			}()

			hc.Ready()
			<-signalsChannel
			logger.Info("Shutting down")

			logger.Info("Shutdown complete")
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())

	flags.SetDefaultHealthCheckPort(builder.RelayDefaultHealthCheckHTTPPort)

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		flags.AddLoggingFlag,
		builder.AddFlags,
		metrics.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func startZipkinHTTPAPI(
	logger *zap.Logger,
	zipkinPort int,
	zipkinSpansHandler collectorApp.ZipkinSpansHandler,
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
