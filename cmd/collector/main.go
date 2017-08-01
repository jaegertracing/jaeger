// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"net"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	basicB "github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/collector/app"
	"github.com/uber/jaeger/cmd/collector/app/builder"
	"github.com/uber/jaeger/cmd/collector/app/zipkin"
	"github.com/uber/jaeger/cmd/flags"
	casFlags "github.com/uber/jaeger/cmd/flags/cassandra"
	"github.com/uber/jaeger/pkg/config"
	"github.com/uber/jaeger/pkg/recoveryhandler"
	jc "github.com/uber/jaeger/thrift-gen/jaeger"
	zc "github.com/uber/jaeger/thrift-gen/zipkincore"
)

func main() {
	logger, _ := zap.NewProduction()
	serviceName := "jaeger-collector"
	casOptions := casFlags.NewOptions("cassandra")

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-collector",
		Short: "Jaeger collector receives and processes traces from Jaeger agents and clients",
		Long: `Jaeger collector receives traces from Jaeger agents and agent and runs them through
				a processing pipeline.`,
		Run: func(cmd *cobra.Command, args []string) {
			casOptions.InitFromViper(v)

			baseMetrics := xkit.Wrap(serviceName, expvar.NewFactory(10))

			builderOpts := new(builder.CollectorOptions).InitFromViper(v)
			sFlags := new(flags.SharedFlags).InitFromViper(v)
			handlerBuilder, err := builder.NewSpanHandlerBuilder(
				builderOpts,
				sFlags,
				basicB.Options.CassandraOption(casOptions.GetPrimary()),
				basicB.Options.LoggerOption(logger),
				basicB.Options.MetricsFactoryOption(baseMetrics),
			)
			if err != nil {
				logger.Fatal("Unable to set up builder", zap.Error(err))
			}
			zipkinSpansHandler, jaegerBatchesHandler, err := handlerBuilder.BuildHandlers()
			if err != nil {
				logger.Fatal("Unable to build span handlers", zap.Error(err))
			}

			ch, err := tchannel.NewChannel(serviceName, &tchannel.ChannelOptions{})
			if err != nil {
				logger.Fatal("Unable to create new TChannel", zap.Error(err))
			}
			server := thrift.NewServer(ch)
			server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
			server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))

			portStr := ":" + strconv.Itoa(builderOpts.CollectorPort)
			listener, err := net.Listen("tcp", portStr)
			if err != nil {
				logger.Fatal("Unable to start listening on channel", zap.Error(err))
			}
			ch.Serve(listener)

			r := mux.NewRouter()
			apiHandler := app.NewAPIHandler(jaegerBatchesHandler)
			apiHandler.RegisterRoutes(r)
			httpPortStr := ":" + strconv.Itoa(builderOpts.CollectorHTTPPort)
			recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

			go startZipkinHTTPAPI(logger, builderOpts.CollectorZipkinHTTPPort, zipkinSpansHandler, recoveryHandler)

			logger.Info("Listening for HTTP traffic", zap.Int("http-port", builderOpts.CollectorHTTPPort))
			if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
				logger.Fatal("Could not launch service", zap.Error(err))
			}
		},
	}

	config.AddFlags(
		v,
		command,
		flags.AddFlags,
		builder.AddFlags,
		casOptions.AddFlags,
	)

	if error := command.Execute(); error != nil {
		logger.Fatal(error.Error())
	}
}

func startZipkinHTTPAPI(
	logger *zap.Logger,
	zipkinPort int,
	zipkinSpansHandler app.ZipkinSpansHandler,
	recoveryHandler func(http.Handler) http.Handler,
) {
	if zipkinPort != 0 {
		r := mux.NewRouter()
		zipkin.NewAPIHandler(zipkinSpansHandler).RegisterRoutes(r)
		httpPortStr := ":" + strconv.Itoa(zipkinPort)
		logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", zipkinPort))

		if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
			logger.Fatal("Could not launch service", zap.Error(err))
		}
	}
}
