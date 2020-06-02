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

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/processor/resourceprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/jaegerreceiver"
	"go.opentelemetry.io/collector/receiver/zipkinreceiver"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/badger"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/grpcplugin"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/memory"
	kafkaRec "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/kafka"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	// Agent component
	Agent ComponentType = iota
	// Collector component
	Collector
	// Ingester component
	Ingester
	// AllInOne component
	AllInOne

	gRPCEndpoint             = "localhost:14250"
	httpThriftBinaryEndpoint = "localhost:14268"
	udpThriftCompactEndpoint = "localhost:6831"
	udpThriftBinaryEndpoint  = "localhost:6832"
)

// ComponentType defines component Jaeger type.
type ComponentType int

// ComponentSettings struct configures generation of the default config
type ComponentSettings struct {
	ComponentType  ComponentType
	Factories      config.Factories
	StorageType    string
	ZipkinHostPort string
}

// CreateDefaultConfig creates default configuration.
func (c ComponentSettings) CreateDefaultConfig() (*configmodels.Config, error) {
	exporters, err := createExporters(c.ComponentType, c.StorageType, c.Factories)
	if err != nil {
		return nil, err
	}
	receivers := createReceivers(c.ComponentType, c.ZipkinHostPort, c.Factories)
	processors := configmodels.Processors{}
	resProcessor := c.Factories.Processors["resource"].CreateDefaultConfig().(*resourceprocessor.Config)
	if len(resProcessor.Labels) > 0 {
		processors[resProcessor.Name()] = resProcessor
	}
	hc := c.Factories.Extensions["health_check"].CreateDefaultConfig()
	return &configmodels.Config{
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Extensions: configmodels.Extensions{hc.Name(): hc},
		Service: configmodels.Service{
			Extensions: []string{hc.Name()},
			Pipelines: configmodels.Pipelines{
				string(configmodels.TracesDataType): {
					InputType:  configmodels.TracesDataType,
					Receivers:  receiverNames(receivers),
					Processors: processorNames(processors),
					Exporters:  exporterNames(exporters),
				},
			},
		},
	}, nil
}

func createReceivers(component ComponentType, zipkinHostPort string, factories config.Factories) configmodels.Receivers {
	if component == Ingester {
		kafkaReceiver := factories.Receivers[kafkaRec.TypeStr].CreateDefaultConfig().(*kafkaRec.Config)
		return configmodels.Receivers{
			kafkaReceiver.Name(): kafkaReceiver,
		}
	}

	jaeger := factories.Receivers["jaeger"].CreateDefaultConfig().(*jaegerreceiver.Config)
	// The CreateDefaultConfig is enabling protocols from flags
	// we do not want to override it here
	if _, ok := jaeger.Protocols["grpc"]; !ok {
		jaeger.Protocols["grpc"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: gRPCEndpoint,
			},
		}
	}
	if _, ok := jaeger.Protocols["thrift_http"]; !ok {
		jaeger.Protocols["thrift_http"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: httpThriftBinaryEndpoint,
			},
		}
	}
	if component == Agent || component == AllInOne {
		enableAgentUDPEndpoints(jaeger)
	}
	recvs := map[string]configmodels.Receiver{
		"jaeger": jaeger,
	}
	if zipkinHostPort != "" && zipkinHostPort != ports.PortToHostPort(0) {
		zipkin := factories.Receivers["zipkin"].CreateDefaultConfig().(*zipkinreceiver.Config)
		zipkin.Endpoint = zipkinHostPort
		recvs["zipkin"] = zipkin
	}
	return recvs
}

func createExporters(component ComponentType, storageTypes string, factories config.Factories) (configmodels.Exporters, error) {
	if component == Agent {
		jaegerExporter := factories.Exporters["jaeger"]
		return configmodels.Exporters{
			"jaeger": jaegerExporter.CreateDefaultConfig(),
		}, nil
	}
	exporters := configmodels.Exporters{}
	for _, s := range strings.Split(storageTypes, ",") {
		switch s {
		case "memory":
			mem := factories.Exporters[memory.TypeStr].CreateDefaultConfig()
			exporters[memory.TypeStr] = mem
		case "badger":
			badg := factories.Exporters[badger.TypeStr].CreateDefaultConfig()
			exporters[badger.TypeStr] = badg
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
			grpcEx := factories.Exporters[grpcplugin.TypeStr].CreateDefaultConfig()
			exporters[grpcplugin.TypeStr] = grpcEx
		default:
			return nil, fmt.Errorf("unknown storage type: %s", s)
		}
	}
	return exporters, nil
}

func enableAgentUDPEndpoints(jaeger *jaegerreceiver.Config) configmodels.Receivers {
	if _, ok := jaeger.Protocols["thrift_compact"]; !ok {
		jaeger.Protocols["thrift_compact"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: udpThriftCompactEndpoint,
			},
		}
	}
	if _, ok := jaeger.Protocols["thrift_binary"]; !ok {
		jaeger.Protocols["thrift_binary"] = &receiver.SecureReceiverSettings{
			ReceiverSettings: configmodels.ReceiverSettings{
				Endpoint: udpThriftBinaryEndpoint,
			},
		}
	}
	recvs := configmodels.Receivers{
		"jaeger": jaeger,
	}
	return recvs
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
