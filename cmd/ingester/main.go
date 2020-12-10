// Copyright (c) 2018 The Jaeger Authors.
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
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
	"github.com/jaegertracing/jaeger/cmd/status"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
)

func main() {
	svc := flags.NewService(ports.IngesterAdminHTTP)

	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-ingester",
		Short: "Jaeger ingester consumes from Kafka and writes to storage.",
		Long:  `Jaeger ingester consumes spans from a particular Kafka topic and writes them to a configured storage.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			metricsFactory := baseFactory.Namespace(metrics.NSOptions{Name: "ingester"})

			storageFactory.InitFromViper(v)
			if err := storageFactory.Initialize(baseFactory, logger); err != nil {
				logger.Fatal("Failed to init storage factory", zap.Error(err))
			}
			spanWriter, err := storageFactory.CreateSpanWriter()
			if err != nil {
				logger.Fatal("Failed to create span writer", zap.Error(err))
			}

			options := app.Options{}
			options.InitFromViper(v)
			consumer, err := builder.CreateConsumer(logger, metricsFactory, spanWriter, options)
			if err != nil {
				logger.Fatal("Unable to create consumer", zap.Error(err))
			}
			consumer.Start()

			svc.RunAndThen(func() {
				if err := options.TLS.Close(); err != nil {
					logger.Error("Failed to close TLS certificates watcher", zap.Error(err))
				}
				if err = consumer.Close(); err != nil {
					logger.Error("Failed to close consumer", zap.Error(err))
				}
				if closer, ok := spanWriter.(io.Closer); ok {
					err := closer.Close()
					if err != nil {
						logger.Error("Failed to close span writer", zap.Error(err))
					}
				}
				if err := storageFactory.Close(); err != nil {
					logger.Error("Failed to close storage factory", zap.Error(err))
				}
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.IngesterAdminHTTP))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		storageFactory.AddFlags,
		app.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
