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
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	spanconsumer "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/pkg/config"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/plugin/storage"
)

func main() {
	var signalsChannel = make(chan os.Signal, 0)
	signal.Notify(signalsChannel, os.Interrupt, syscall.SIGTERM)

	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-ingester",
		Short: "Jaeger ingester consumes from Kafka and writes to storage",
		Long:  `Jaeger ingester consumes spans from a particular Kafka topic and writes them to all configured storage types.`,
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
			spanProcessor := processor.NewSpanProcessor(spanWriter)

			metricsFactory := baseFactory.Namespace("ingester", nil)

			// Will be replaced with values from viper
			consumerConfig := spanconsumer.Config{
				Brokers:     []string{"127.0.0.1:9092"},
				Topic:       "jaeger-spans",
				GroupID:     "jaeger-ingester",
				Parallelism: 1000,
			}
			consumerParams := spanconsumer.Params{
				Config:    consumerConfig,
				Processor: spanProcessor,
				Factory:   metricsFactory,
				Logger:    logger,
			}
			consumer, err := spanconsumer.New(consumerParams)
			if err != nil {
				logger.Fatal("Unable to set up consumer", zap.Error(err))
			}
			consumer.Start()

			select {
			case <-signalsChannel:
				err := consumer.Close()
				if err != nil {
					logger.Error("Failed to close span writer", zap.Error(err))
				}
				logger.Info("Jaeger Ingester is finishing")
			}
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())

	// todo
	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddFlags,
		storageFactory.AddFlags,
		pMetrics.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
