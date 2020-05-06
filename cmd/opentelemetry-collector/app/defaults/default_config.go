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
	"github.com/open-telemetry/opentelemetry-collector/extension/healthcheckextension"
	"github.com/open-telemetry/opentelemetry-collector/processor/resourceprocessor"
	"github.com/open-telemetry/opentelemetry-collector/receiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/zipkinreceiver"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	gRPCEndpoint             = "localhost:14250"
	httpThriftBinaryEndpoint = "localhost:14268"
	udpThriftCompactEndpoint = "localhost:6831"
	udpThriftBinaryEndpoint  = "localhost:6832"
)

// CollectorConfig creates default collector configuration.
// It enables default Jaeger receivers, processors and exporters.
func CollectorConfig(storageType string, zipkinHostPort string, factories config.Factories) (*configmodels.Config, error) {
	exporters, err := createExporters(storageType, factories)
	if err != nil {
		return nil, err
	}
	expTypes := []string{}
	for _, v := range exporters {
		expTypes = append(expTypes, string(v.Type()))
	}
	receivers := createCollectorReceivers(zipkinHostPort, factories)
	recTypes := []string{}
	for _, v := range receivers {
		recTypes = append(recTypes, string(v.Type()))
	}
	hc := factories.Extensions["health_check"].CreateDefaultConfig()
	resProcessor := factories.Processors["resource"].CreateDefaultConfig()
	return &configmodels.Config{
		Receivers:  receivers,
		Processors: configmodels.Processors{"resource": resProcessor},
		Exporters:  exporters,
		Extensions: configmodels.Extensions{"health_check": hc},
		Service: configmodels.Service{
			Extensions: []string{"health_check"},
			Pipelines: configmodels.Pipelines{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  recTypes,
					Processors: []string{"resource"},
					Exporters:  expTypes,
				},
			},
		},
	}, nil
}

func createCollectorReceivers(zipkinHostPort string, factories config.Factories) configmodels.Receivers {
	jaeger := factories.Receivers["jaeger"].CreateDefaultConfig().(*jaegerreceiver.Config)
	// TODO load and serve sampling strategies
	// TODO bind sampling strategies file
	jaeger.Protocols = map[string]*receiver.SecureReceiverSettings{
		"grpc": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: gRPCEndpoint,
			},
		},
		"thrift_http": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: httpThriftBinaryEndpoint,
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

// AgentConfig creates default agent configuration.
// It enables Jaeger receiver with UDP endpoints and Jaeger exporter.
func AgentConfig(factories config.Factories) *configmodels.Config {
	jaegerExporter := factories.Exporters["jaeger"]
	hc := factories.Extensions["health_check"].CreateDefaultConfig().(*healthcheckextension.Config)
	processors := configmodels.Processors{}
	resProcessor := factories.Processors["resource"].CreateDefaultConfig().(*resourceprocessor.Config)
	if len(resProcessor.Labels) > 0 {
		processors[resProcessor.Name()] = resProcessor
	}
	return &configmodels.Config{
		Receivers:  createAgentReceivers(factories),
		Processors: processors,
		Exporters:  configmodels.Exporters{"jaeger": jaegerExporter.CreateDefaultConfig()},
		Extensions: configmodels.Extensions{"health_check": hc},
		Service: configmodels.Service{
			Extensions: []string{"health_check"},
			Pipelines: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  []string{"jaeger"},
					Processors: processorNames(processors),
					Exporters:  []string{"jaeger"},
				},
			},
		},
	}
}

func createAgentReceivers(factories config.Factories) configmodels.Receivers {
	jaeger := factories.Receivers["jaeger"].CreateDefaultConfig().(*jaegerreceiver.Config)
	jaeger.Protocols = map[string]*receiver.SecureReceiverSettings{
		"thrift_compact": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: udpThriftCompactEndpoint,
			},
		},
		"thrift_binary": {
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: udpThriftBinaryEndpoint,
			},
		},
	}
	recvs := map[string]configmodels.Receiver{
		"jaeger": jaeger,
	}
	return recvs
}

func processorNames(processors configmodels.Processors) []string {
	var names []string
	for _, v := range processors {
		names = append(names, v.Name())
	}
	return names
}
