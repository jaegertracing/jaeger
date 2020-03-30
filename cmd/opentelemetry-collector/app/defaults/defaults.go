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
	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/defaults"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	storageCassandra "github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	storageEs "github.com/jaegertracing/jaeger/plugin/storage/es"
	storageKafka "github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

// Components creates default and Jaeger factories
func Components(v *viper.Viper) config.Factories {
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

	factories, _ := defaults.Components()
	factories.Exporters[kafkaExp.Type()] = kafkaExp
	factories.Exporters[cassandraExp.Type()] = cassandraExp
	factories.Exporters[esExp.Type()] = esExp
	return factories
}
