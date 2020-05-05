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

package defaults

import (
	"flag"

	"github.com/open-telemetry/opentelemetry-collector/config"
	otelJaegerEexporter "github.com/open-telemetry/opentelemetry-collector/exporter/jaegerexporter"
	otelJaegerreceiver "github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector/service/defaultcomponents"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/jaegerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/jaegerreceiver"
	storageCassandra "github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	storageEs "github.com/jaegertracing/jaeger/plugin/storage/es"
	storageKafka "github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

// Components creates default and Jaeger factories
func Components(v *viper.Viper) config.Factories {
	// Add flags to viper to make the default values available.
	// We have to add all storage flags to viper because any exporter can be specified in the OTEL config file.
	// OTEL collector creates default configurations for all factories to verify they can be created.
	addDefaultValuesToViper(v)
	kafkaExp := kafka.Factory{OptionsFactory: func() *storageKafka.Options {
		opts := kafka.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	cassandraExp := cassandra.Factory{OptionsFactory: func() *storageCassandra.Options {
		opts := cassandra.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}
	esExp := elasticsearch.Factory{OptionsFactory: func() *storageEs.Options {
		opts := elasticsearch.DefaultOptions()
		opts.InitFromViper(v)
		return opts
	}}

	factories, _ := defaultcomponents.Components()
	factories.Exporters[kafkaExp.Type()] = kafkaExp
	factories.Exporters[cassandraExp.Type()] = cassandraExp
	factories.Exporters[esExp.Type()] = esExp

	jaegerRec := factories.Receivers["jaeger"].(*otelJaegerreceiver.Factory)
	factories.Receivers["jaeger"] = &jaegerreceiver.Factory{
		Wrapped: jaegerRec,
		Viper:   v,
	}
	jaegerExp := factories.Exporters["jaeger"].(*otelJaegerEexporter.Factory)
	factories.Exporters["jaeger"] = &jaegerexporter.Factory{
		Wrapped: jaegerExp,
		Viper:   v,
	}
	return factories
}

// addDefaultValuesToViper adds Jaeger storage flags to viper to make the default values available.
func addDefaultValuesToViper(v *viper.Viper) {
	flagSet := &flag.FlagSet{}
	kafka.DefaultOptions().AddFlags(flagSet)
	elasticsearch.DefaultOptions().AddFlags(flagSet)
	cassandra.DefaultOptions().AddFlags(flagSet)
	pflagSet := &pflag.FlagSet{}
	pflagSet.AddGoFlagSet(flagSet)
	v.BindPFlags(pflagSet)
}
