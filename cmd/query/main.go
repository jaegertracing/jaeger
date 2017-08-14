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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	"go.uber.org/zap"

	basicB "github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/flags"
	casFlags "github.com/uber/jaeger/cmd/flags/cassandra"
	esFlags "github.com/uber/jaeger/cmd/flags/es"
	"github.com/uber/jaeger/cmd/query/app"
	"github.com/uber/jaeger/cmd/query/app/builder"
	"github.com/uber/jaeger/pkg/config"
	"github.com/uber/jaeger/pkg/healthcheck"
	"github.com/uber/jaeger/pkg/recoveryhandler"
)

func main() {
	var serverChannel = make(chan os.Signal, 0)
	signal.Notify(serverChannel, os.Interrupt, syscall.SIGTERM)

	logger, _ := zap.NewProduction()
	casOptions := casFlags.NewOptions("cassandra", "cassandra.archive")
	esOptions := esFlags.NewOptions("es", "es.archive")
	v := viper.New()

	var command = &cobra.Command{
		Use:   "jaeger-query",
		Short: "Jaeger query is a service to access tracing data",
		Long:  `Jaeger query is a service to access tracing data and host UI.`,
		Run: func(cmd *cobra.Command, args []string) {
			casOptions.InitFromViper(v)
			esOptions.InitFromViper(v)
			queryOpts := new(builder.QueryOptions).InitFromViper(v)
			sFlags := new(flags.SharedFlags).InitFromViper(v)

			hc, err := healthcheck.Serve(http.StatusServiceUnavailable, queryOpts.QueryHealthCheckHTTPPort, logger)
			if err != nil {
				logger.Fatal("Could not start the health check server.", zap.Error(err))
			}

			metricsFactory := xkit.Wrap("jaeger-query", expvar.NewFactory(10))

			storageBuild, err := builder.NewStorageBuilder(
				sFlags.SpanStorage.Type,
				sFlags.DependencyStorage.DataFrequency,
				basicB.Options.LoggerOption(logger),
				basicB.Options.MetricsFactoryOption(metricsFactory),
				basicB.Options.CassandraSessionOption(casOptions.GetPrimary()),
				basicB.Options.ElasticClientOption(esOptions.GetPrimary()),
			)
			if err != nil {
				logger.Fatal("Failed to init storage builder", zap.Error(err))
			}
			rHandler := app.NewAPIHandler(
				storageBuild.SpanReader,
				storageBuild.DependencyReader,
				app.HandlerOptions.Prefix(queryOpts.QueryPrefix),
				app.HandlerOptions.Logger(logger))
			sHandler := app.NewStaticAssetsHandler(queryOpts.QueryStaticAssets)
			r := mux.NewRouter()
			rHandler.RegisterRoutes(r)
			sHandler.RegisterRoutes(r)
			portStr := ":" + strconv.Itoa(queryOpts.QueryPort)
			recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

			go func() {
				logger.Info("Starting jaeger-query HTTP server", zap.Int("port", queryOpts.QueryPort))
				if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
					logger.Fatal("Could not launch service", zap.Error(err))
				}
				hc.Set(http.StatusInternalServerError)
			}()

			hc.Ready()

			select {
			case <-serverChannel:
				logger.Info("Jaeger Query is finishing")
			}
		},
	}

	config.AddFlags(
		v,
		command,
		flags.AddFlags,
		casOptions.AddFlags,
		esOptions.AddFlags,
		builder.AddFlags,
	)

	if error := command.Execute(); error != nil {
		logger.Fatal(error.Error())
	}
}
