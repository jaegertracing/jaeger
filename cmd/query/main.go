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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	"go.uber.org/zap"

	basicB "github.com/jaegertracing/jaeger/cmd/builder"
	"github.com/jaegertracing/jaeger/cmd/flags"
	casFlags "github.com/jaegertracing/jaeger/cmd/flags/cassandra"
	esFlags "github.com/jaegertracing/jaeger/cmd/flags/es"
	dashFlags "github.com/jaegertracing/jaeger/cmd/flags/dashbase"
	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/builder"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	var serverChannel = make(chan os.Signal, 0)
	signal.Notify(serverChannel, os.Interrupt, syscall.SIGTERM)

	casOptions := casFlags.NewOptions("cassandra", "cassandra.archive")
	esOptions := esFlags.NewOptions("es", "es.archive")
	dashOptions := dashFlags.NewOptions("dashbase")
	v := viper.New()

	var command = &cobra.Command{
		Use:   "jaeger-query",
		Short: "Jaeger query is a service to access tracing data",
		Long:  `Jaeger query is a service to access tracing data and host UI.`,
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

			casOptions.InitFromViper(v)
			esOptions.InitFromViper(v)
			dashOptions.InitFromViper(v)

			queryOpts := new(builder.QueryOptions).InitFromViper(v)

			hc, err := healthcheck.Serve(http.StatusServiceUnavailable, queryOpts.HealthCheckHTTPPort, logger)
			if err != nil {
				logger.Fatal("Could not start the health check server.", zap.Error(err))
			}

			metricsFactory := xkit.Wrap("jaeger-query", expvar.NewFactory(10))

			tracer, closer, err := jaegerClientConfig.Configuration{
				Sampler: &jaegerClientConfig.SamplerConfig{
					Type:  "probabilistic",
					Param: 1.0,
				},
				RPCMetrics: true,
			}.New("jaeger-query", jaegerClientConfig.Metrics(metricsFactory))
			if err != nil {
				logger.Fatal("Failed to initialize tracer", zap.Error(err))
			}
			defer closer.Close()

			storageBuild, err := builder.NewStorageBuilder(
				sFlags.SpanStorage.Type,
				sFlags.DependencyStorage.DataFrequency,
				basicB.Options.LoggerOption(logger),
				basicB.Options.MetricsFactoryOption(metricsFactory),
				basicB.Options.CassandraSessionOption(casOptions.GetPrimary()),
				basicB.Options.ElasticClientOption(esOptions.GetPrimary()),
				basicB.Options.DashbaseOption(dashOptions.GetPrimary()),
			)
			if err != nil {
				logger.Fatal("Failed to init storage builder", zap.Error(err))
			}

			apiHandler := app.NewAPIHandler(
				storageBuild.SpanReader,
				storageBuild.DependencyReader,
				app.HandlerOptions.Prefix(queryOpts.Prefix),
				app.HandlerOptions.Logger(logger),
				app.HandlerOptions.Tracer(tracer))
			r := mux.NewRouter()
			apiHandler.RegisterRoutes(r)
			registerStaticHandler(r, logger, queryOpts)
			portStr := ":" + strconv.Itoa(queryOpts.Port)
			recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

			go func() {
				logger.Info("Starting jaeger-query HTTP server", zap.Int("port", queryOpts.Port))
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
			return nil
		},
	}

	command.AddCommand(version.Command())

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		casOptions.AddFlags,
		esOptions.AddFlags,
		builder.AddFlags,
		dashOptions.AddFlags,
	)

	if error := command.Execute(); error != nil {
		fmt.Println(error.Error())
		os.Exit(1)
	}
}

func registerStaticHandler(r *mux.Router, logger *zap.Logger, qOpts *builder.QueryOptions) {
	staticHandler, err := app.NewStaticAssetsHandler(qOpts.StaticAssets, qOpts.UIConfig)
	if err != nil {
		logger.Fatal("Could not create static assets handler", zap.Error(err))
	}
	if staticHandler != nil {
		staticHandler.RegisterRoutes(r)
	} else {
		logger.Info("Static handler is not registered")
	}
}
