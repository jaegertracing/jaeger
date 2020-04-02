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
	"strings"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/processor/batchprocessor"
	"github.com/open-telemetry/opentelemetry-collector/receiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
)

// DefaultConfig creates default configuration.
// It enabled default Jaeger receivers, processors and exporters.
func DefaultConfig(storageType string, factories config.Factories) *configmodels.Config {
	exporters := createExporters(storageType, factories)
	types := []string{}
	for _, v := range exporters {
		types = append(types, v.Type())
	}
	return &configmodels.Config{
		Receivers:  createReceivers(factories),
		Exporters:  exporters,
		Processors: createProcessors(factories),
		Service: configmodels.Service{
			Pipelines: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  []string{"jaeger"},
					Exporters:  types,
					Processors: []string{"batch"},
				},
			},
		},
	}
}

func createReceivers(factories config.Factories) configmodels.Receivers {
	rec := factories.Receivers["jaeger"].CreateDefaultConfig().(*jaegerreceiver.Config)
	// TODO load and serve sampling strategies
	// TODO bind sampling strategies file
	rec.Protocols = map[string]*receiver.SecureReceiverSettings{
		"grpc": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: "localhost:14250",
			},
		},
		"thrift_http": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: "localhost:14268",
			},
		},
		"thrift_compact": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: "localhost:6831",
			},
		},
		"thrift_binary": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: "localhost:6832",
			},
		},
	}
	return map[string]configmodels.Receiver{
		"jaeger": rec,
	}
}

func createExporters(storageTypes string, factories config.Factories) configmodels.Exporters {
	exporters := configmodels.Exporters{}
	for _, storage := range strings.Split(storageTypes, ",") {
		if storage == "elasticsearch" {
			es := factories.Exporters[elasticsearch.TypeStr].CreateDefaultConfig()
			exporters[elasticsearch.TypeStr] = es
		} else if storage == "cassandra" {
			cass := factories.Exporters[cassandra.TypeStr].CreateDefaultConfig()
			exporters[cassandra.TypeStr] = cass
		} else if storage == "kafka" {
			kaf := factories.Exporters[kafka.TypeStr].CreateDefaultConfig()
			exporters[kafka.TypeStr] = kaf
		}
	}
	return exporters
}

func createProcessors(factories config.Factories) configmodels.Processors {
	batch := factories.Processors["batch"].CreateDefaultConfig().(*batchprocessor.Config)
	return map[string]configmodels.Processor{
		"batch": batch,
	}
}
