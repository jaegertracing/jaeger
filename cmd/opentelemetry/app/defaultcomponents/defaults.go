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

package defaultcomponents

import (
	"flag"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config"
	otelJaegerExporter "go.opentelemetry.io/collector/exporter/jaegerexporter"
	otelResourceProcessor "go.opentelemetry.io/collector/processor/resourceprocessor"
	otelJaegerReceiver "go.opentelemetry.io/collector/receiver/jaegerreceiver"
	otelZipkinReceiver "go.opentelemetry.io/collector/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/service/defaultcomponents"

	ingesterApp "github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/badgerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/cassandraexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/grpcpluginexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/jaegerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/kafkaexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/memoryexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/processor/resourceprocessor"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/jaegerreceiver"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/kafkareceiver"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/zipkinreceiver"
	badgerStorage "github.com/jaegertracing/jaeger/plugin/storage/badger"
	cassandraStorage "github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	esStorage "github.com/jaegertracing/jaeger/plugin/storage/es"
	grpcStorage "github.com/jaegertracing/jaeger/plugin/storage/grpc"
	kafkaStorage "github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

// Components creates default and Jaeger factories
func Components(v *viper.Viper) config.Factories {
	// Add flags to viper to make the default values available.
	// We have to add all storage flags to viper because any exporter can be specified in the OTEL config file.
	// OTEL collector creates default configurations for all factories to verify they can be created.
	addDefaultValuesToViper(v)
	kafkaExp := &kafkaexporter.Factory{OptionsFactory: func() *kafkaStorage.Options {
		opts := kafkaexporter.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	cassandraExp := &cassandraexporter.Factory{OptionsFactory: func() *cassandraStorage.Options {
		opts := cassandraexporter.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	esExp := &elasticsearchexporter.Factory{OptionsFactory: func() *esStorage.Options {
		opts := elasticsearchexporter.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	grpcExp := &grpcpluginexporter.Factory{OptionsFactory: func() *grpcStorage.Options {
		opts := grpcpluginexporter.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	memoryExp := memoryexporter.NewFactory(v)
	badgerExp := badgerexporter.NewFactory(func() *badgerStorage.Options {
		opts := badgerexporter.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	})
	kafkaRec := &kafkareceiver.Factory{OptionsFactory: func() *ingesterApp.Options {
		opts := kafkareceiver.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}

	factories, _ := defaultcomponents.Components()
	factories.Exporters[kafkaExp.Type()] = kafkaExp
	factories.Exporters[cassandraExp.Type()] = cassandraExp
	factories.Exporters[esExp.Type()] = esExp
	factories.Exporters[grpcExp.Type()] = grpcExp
	factories.Exporters[memoryExp.Type()] = memoryExp
	factories.Exporters[badgerExp.Type()] = badgerExp
	factories.Receivers[kafkaRec.Type()] = kafkaRec

	jaegerRec := factories.Receivers["jaeger"].(*otelJaegerReceiver.Factory)
	factories.Receivers["jaeger"] = &jaegerreceiver.Factory{
		Wrapped: jaegerRec,
		Viper:   v,
	}
	jaegerExp := factories.Exporters["jaeger"].(*otelJaegerExporter.Factory)
	factories.Exporters["jaeger"] = &jaegerexporter.Factory{
		Wrapped: jaegerExp,
		Viper:   v,
	}
	zipkinRec := factories.Receivers["zipkin"].(*otelZipkinReceiver.Factory)
	factories.Receivers["zipkin"] = &zipkinreceiver.Factory{
		Wrapped: zipkinRec,
		Viper:   v,
	}

	resourceProc := factories.Processors["resource"].(*otelResourceProcessor.Factory)
	factories.Processors["resource"] = &resourceprocessor.Factory{
		Wrapped: resourceProc,
		Viper:   v,
	}
	return factories
}

// addDefaultValuesToViper adds Jaeger storage flags to viper to make the default values available.
func addDefaultValuesToViper(v *viper.Viper) {
	flagSet := &flag.FlagSet{}
	kafkaexporter.DefaultOptions().AddFlags(flagSet)
	elasticsearchexporter.DefaultOptions().AddFlags(flagSet)
	cassandraexporter.DefaultOptions().AddFlags(flagSet)
	pflagSet := &pflag.FlagSet{}
	pflagSet.AddGoFlagSet(flagSet)
	v.BindPFlags(pflagSet)
}
