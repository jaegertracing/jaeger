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
	"fmt"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/processor/batchprocessor"
	"github.com/open-telemetry/opentelemetry-collector/receiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/zipkinreceiver"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/ports"
)

// Config creates default configuration.
// It enables default Jaeger receivers, processors and exporters.
func Config(storageType string, zipkinHostPort string, factories config.Factories) (*configmodels.Config, error) {
	exporters, err := createExporters(storageType, factories)
	if err != nil {
		return nil, err
	}
	expTypes := []string{}
	for _, v := range exporters {
		expTypes = append(expTypes, string(v.Type()))
	}
	receivers := createReceivers(zipkinHostPort, factories)
	recTypes := []string{}
	for _, v := range receivers {
		recTypes = append(recTypes, string(v.Type()))
	}
	hc := factories.Extensions["health_check"].CreateDefaultConfig()
	return &configmodels.Config{
		Receivers:  receivers,
		Exporters:  exporters,
		Processors: createProcessors(factories),
		Extensions: configmodels.Extensions{"health_check": hc},
		Service: configmodels.Service{
			Extensions: []string{"health_check"},
			Pipelines: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  recTypes,
					Exporters:  expTypes,
					Processors: []string{"batch"},
				},
			},
		},
	}, nil
}

func createReceivers(zipkinHostPort string, factories config.Factories) configmodels.Receivers {
	jaeger := factories.Receivers["jaeger"].CreateDefaultConfig().(*jaegerreceiver.Config)
	// TODO load and serve sampling strategies
	// TODO bind sampling strategies file
	jaeger.Protocols = map[string]*receiver.SecureReceiverSettings{
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
	recvs := map[string]configmodels.Receiver{
		"jaeger": jaeger,
	}
	if zipkinHostPort != ports.PortToHostPort(0) {
		zipkin := factories.Receivers["zipkin"].CreateDefaultConfig().(*zipkinreceiver.Config)
		zipkin.Endpoint = zipkinHostPort
		recvs["zipkin"] = zipkin
	}
	return recvs
}

func createExporters(storageTypes string, factories config.Factories) (configmodels.Exporters, error) {
	exporters := configmodels.Exporters{}
	for _, s := range strings.Split(storageTypes, ",") {
		switch s {
		case "cassandra":
			cass := factories.Exporters[cassandra.TypeStr].CreateDefaultConfig()
			exporters[cassandra.TypeStr] = cass
		case "elasticsearch":
			es := factories.Exporters[elasticsearch.TypeStr].CreateDefaultConfig()
			exporters[elasticsearch.TypeStr] = es
		case "kafka":
			kaf := factories.Exporters[kafka.TypeStr].CreateDefaultConfig()
			exporters[kafka.TypeStr] = kaf
		default:
			return nil, fmt.Errorf("unknown storage type: %s", s)
		}
	}
	return exporters, nil
}

func createProcessors(factories config.Factories) configmodels.Processors {
	batch := factories.Processors["batch"].CreateDefaultConfig().(*batchprocessor.Config)
	return map[string]configmodels.Processor{
		"batch": batch,
	}
}
