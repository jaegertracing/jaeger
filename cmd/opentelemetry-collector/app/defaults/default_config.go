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
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/grpcplugin"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	kafkaRec "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/kafka"
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
	receivers := createCollectorReceivers(zipkinHostPort, factories)
	hc := factories.Extensions["health_check"].CreateDefaultConfig()
	processors := configmodels.Processors{}
	resProcessor := factories.Processors["resource"].CreateDefaultConfig().(*resourceprocessor.Config)
	if len(resProcessor.Labels) > 0 {
		processors[resProcessor.Name()] = resProcessor
	}
	return &configmodels.Config{
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Extensions: configmodels.Extensions{"health_check": hc},
		Service: configmodels.Service{
			Extensions: []string{"health_check"},
			Pipelines: configmodels.Pipelines{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  receiverNames(receivers),
					Processors: processorNames(processors),
					Exporters:  exporterNames(exporters),
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
		case "grpc-plugin":
			grpcEx := factories.Exporters[grpc.TypeStr].CreateDefaultConfig()
			exporters[grpc.TypeStr] = grpcEx
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
	exporters := configmodels.Exporters{
		"jaeger": jaegerExporter.CreateDefaultConfig(),
	}
	hc := factories.Extensions["health_check"].CreateDefaultConfig().(*healthcheckextension.Config)
	processors := configmodels.Processors{}
	resProcessor := factories.Processors["resource"].CreateDefaultConfig().(*resourceprocessor.Config)
	if len(resProcessor.Labels) > 0 {
		processors[resProcessor.Name()] = resProcessor
	}
	receivers := createAgentReceivers(factories)
	return &configmodels.Config{
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Extensions: configmodels.Extensions{"health_check": hc},
		Service: configmodels.Service{
			Extensions: []string{"health_check"},
			Pipelines: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  receiverNames(receivers),
					Processors: processorNames(processors),
					Exporters:  exporterNames(exporters),
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
	recvs := configmodels.Receivers{
		"jaeger": jaeger,
	}
	return recvs
}

// IngesterConfig creates default ingester configuration.
// It enables Jaeger kafka receiver and storage backend.
func IngesterConfig(storageType string, factories config.Factories) (*configmodels.Config, error) {
	exporters, err := createExporters(storageType, factories)
	if err != nil {
		return nil, err
	}
	kafkaReceiver := factories.Receivers[kafkaRec.TypeStr].CreateDefaultConfig().(*kafkaRec.Config)
	receivers := configmodels.Receivers{
		kafkaReceiver.Name(): kafkaReceiver,
	}
	hc := factories.Extensions["health_check"].CreateDefaultConfig()
	return &configmodels.Config{
		Receivers:  receivers,
		Exporters:  exporters,
		Extensions: configmodels.Extensions{"health_check": hc},
		Service: configmodels.Service{
			Extensions: []string{"health_check"},
			Pipelines: configmodels.Pipelines{
				"traces": {
					InputType: configmodels.TracesDataType,
					Receivers: receiverNames(receivers),
					Exporters: exporterNames(exporters),
				},
			},
		},
	}, nil
}

func receiverNames(receivers configmodels.Receivers) []string {
	var names []string
	for _, v := range receivers {
		names = append(names, v.Name())
	}
	return names
}

func processorNames(processors configmodels.Processors) []string {
	var names []string
	for _, v := range processors {
		names = append(names, v.Name())
	}
	return names
}

func exporterNames(exporters configmodels.Exporters) []string {
	var names []string
	for _, v := range exporters {
		names = append(names, v.Name())
	}
	return names
}
