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

package defaultconfig

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/resourceprocessor"
	"go.opentelemetry.io/collector/receiver/jaegerreceiver"
	"go.opentelemetry.io/collector/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/builder"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/badgerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/cassandraexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/grpcpluginexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/kafkaexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/memoryexporter"
	jaegerresource "github.com/jaegertracing/jaeger/cmd/opentelemetry/app/processor/resourceprocessor"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/kafkareceiver"
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
	ComponentType ComponentType
	Factories     component.Factories
	StorageType   string
}

// DefaultConfigFactory returns a service.ConfigFactory that merges jaeger and otel configs
func (c *ComponentSettings) DefaultConfigFactory(jaegerViper *viper.Viper) service.ConfigFactory {
	return func(otelViper *viper.Viper, cmd *cobra.Command, f component.Factories) (*configmodels.Config, error) {
		cfg, err := c.createDefaultConfig()
		if err != nil {
			return nil, err
		}
		if len(builder.GetConfigFile()) > 0 {
			otelCfg, err := service.FileLoaderConfigFactory(otelViper, cmd, f)
			if err != nil {
				return nil, err
			}
			err = MergeConfigs(cfg, otelCfg)
			if err != nil {
				return nil, err
			}
		}

		return cfg, nil
	}
}

// createDefaultConfig creates default configuration.
func (c ComponentSettings) createDefaultConfig() (*configmodels.Config, error) {
	exporters, err := createExporters(c.ComponentType, c.StorageType, c.Factories)
	if err != nil {
		return nil, err
	}
	receivers := createReceivers(c.ComponentType, c.Factories)
	processors, processorNames := createProcessors(c.Factories)
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
					Processors: processorNames,
					Exporters:  exporterNames(exporters),
				},
			},
		},
	}, nil
}

func createProcessors(factories component.Factories) (configmodels.Processors, []string) {
	processors := configmodels.Processors{}
	var names []string
	resFactory := factories.Processors["resource"].(*jaegerresource.Factory)
	if len(resFactory.GetTags()) > 0 {
		resource := factories.Processors["resource"].CreateDefaultConfig().(*resourceprocessor.Config)
		processors[resource.Name()] = resource
		names = append(names, resource.Name())
	}
	batch := factories.Processors["batch"].CreateDefaultConfig().(*batchprocessor.Config)
	processors[batch.Name()] = batch
	names = append(names, batch.Name())
	return processors, names
}

func createReceivers(component ComponentType, factories component.Factories) configmodels.Receivers {
	if component == Ingester {
		kafkaReceiver := factories.Receivers[kafkareceiver.TypeStr].CreateDefaultConfig()
		return configmodels.Receivers{
			kafkaReceiver.Name(): kafkaReceiver,
		}
	}

	jaeger := factories.Receivers["jaeger"].CreateDefaultConfig().(*jaegerreceiver.Config)
	// The CreateDefaultConfig is enabling protocols from flags
	// we do not want to override it here
	if jaeger.GRPC == nil {
		jaeger.GRPC = &configgrpc.GRPCServerSettings{
			NetAddr: confignet.NetAddr{
				Endpoint: gRPCEndpoint,
			},
		}
	}
	if jaeger.ThriftHTTP == nil {
		jaeger.ThriftHTTP = &confighttp.HTTPServerSettings{
			Endpoint: httpThriftBinaryEndpoint,
		}
	}
	if component == Agent || component == AllInOne {
		enableAgentUDPEndpoints(jaeger)
	}
	recvs := map[string]configmodels.Receiver{
		"jaeger": jaeger,
		"otlp":   factories.Receivers["otlp"].CreateDefaultConfig(),
	}
	zipkin := factories.Receivers["zipkin"].CreateDefaultConfig().(*zipkinreceiver.Config)
	if zipkin.Endpoint != "" {
		recvs["zipkin"] = zipkin
	}
	return recvs
}

func createExporters(component ComponentType, storageTypes string, factories component.Factories) (configmodels.Exporters, error) {
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
			mem := factories.Exporters[memoryexporter.TypeStr].CreateDefaultConfig()
			exporters[memoryexporter.TypeStr] = mem
		case "badger":
			badg := factories.Exporters[badgerexporter.TypeStr].CreateDefaultConfig()
			exporters[badgerexporter.TypeStr] = badg
		case "cassandra":
			cass := factories.Exporters[cassandraexporter.TypeStr].CreateDefaultConfig()
			exporters[cassandraexporter.TypeStr] = cass
		case "elasticsearch":
			es := factories.Exporters[elasticsearchexporter.TypeStr].CreateDefaultConfig()
			exporters[elasticsearchexporter.TypeStr] = es
		case "kafka":
			kaf := factories.Exporters[kafkaexporter.TypeStr].CreateDefaultConfig()
			exporters["kafka"] = kaf
		case "grpc-plugin":
			grpcEx := factories.Exporters[grpcpluginexporter.TypeStr].CreateDefaultConfig()
			exporters[grpcpluginexporter.TypeStr] = grpcEx
		default:
			return nil, fmt.Errorf("unknown storage type: %s", s)
		}
	}
	return exporters, nil
}

func enableAgentUDPEndpoints(jaeger *jaegerreceiver.Config) {
	if jaeger.ThriftCompact == nil {
		jaeger.ThriftCompact = &jaegerreceiver.ProtocolUDP{
			Endpoint:        udpThriftCompactEndpoint,
			ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
		}
	}
	if jaeger.ThriftBinary == nil {
		jaeger.ThriftBinary = &jaegerreceiver.ProtocolUDP{
			Endpoint:        udpThriftBinaryEndpoint,
			ServerConfigUDP: jaegerreceiver.DefaultServerConfigUDP(),
		}
	}
}

func receiverNames(receivers configmodels.Receivers) []string {
	var names []string
	for _, v := range receivers {
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
