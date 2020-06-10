// Copyright (c) 2020 The Jaeger Authors.
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
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/benchmark/app"
	"github.com/jaegertracing/jaeger/cmd/benchmark/generator"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func benchmarkFactory(spanWriter spanstore.Writer, traceGenerator *generator.TraceGenerator, options app.Options) func(b *testing.B) {
	return func(b *testing.B) {
		b.StopTimer()
		var spans []model.Span
		for i := 0; i < options.TracesNumber; i++ {
			trace := traceGenerator.Generate(options.SpanMinNumber, options.SpanMaxNumber)
			spans = append(spans, trace...)
		}
		error := 0
		b.StartTimer()
		for i := 0; i < b.N; i++ {
			err := spanWriter.WriteSpan(&spans[i])
			if err != nil {
				error++
			}
		}
		b.StopTimer()
	}
}

func main() {
	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		log.Fatalf("Cannot initialize storage factory: %v", err)
	}
	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-benchmark",
		Short: "Benchmark utilities for jaeger.",
		Long:  `Benchmark tests for jaeger.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spanStorageType := os.Getenv(storage.SpanStorageTypeEnvVar)

			storageFactory.InitFromViper(v)
			err := storageFactory.Initialize(metrics.NullFactory, zap.NewNop())
			if err != nil {
				log.Fatal("Failed to create storage factory", err)

			}
			spanWriter, err := storageFactory.CreateSpanWriter()
			if err != nil {
				log.Fatal("Failed to create span writer", err)
			}
			options := app.Options{}
			options.InitFromViper(v)

			traceGenerator := generator.NewSpanGenerator()
			traceGenerator.Init()
			benchFunc := benchmarkFactory(spanWriter, traceGenerator, options)
			results := testing.Benchmark(benchFunc)
			log.Printf("Benchmark-" + spanStorageType+":" + results.String())
			return nil
		},
	}

	config.AddFlags(
		v,
		command,
		storageFactory.AddFlags,
		app.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
