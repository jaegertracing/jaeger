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
	"sort"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/exporter/jaegerexporter"
	"github.com/open-telemetry/opentelemetry-collector/processor/resourceprocessor"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	grpcExp "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	kafkaRec "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/kafka"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/ports"
)

func TestDefaultCollectorConfig(t *testing.T) {
	disabledHostPort := ports.PortToHostPort(0)
	tests := []struct {
		storageType    string
		zipkinHostPort string
		exporterTypes  []string
		pipeline       configmodels.Pipelines
		err            string
		config         map[string]interface{}
	}{
		{
			storageType:    "elasticsearch",
			zipkinHostPort: disabledHostPort,
			exporterTypes:  []string{elasticsearch.TypeStr},
			pipeline: configmodels.Pipelines{
				"traces": {
					InputType: configmodels.TracesDataType,
					Receivers: []string{"jaeger"},
					Exporters: []string{elasticsearch.TypeStr},
				},
			},
		},
		{
			storageType:    "cassandra",
			zipkinHostPort: disabledHostPort,
			exporterTypes:  []string{cassandra.TypeStr},
			pipeline: configmodels.Pipelines{
				"traces": {
					InputType: configmodels.TracesDataType,
					Receivers: []string{"jaeger"},
					Exporters: []string{cassandra.TypeStr},
				},
			},
		},
		{
			storageType:    "kafka",
			zipkinHostPort: disabledHostPort,
			exporterTypes:  []string{kafka.TypeStr},
			pipeline: configmodels.Pipelines{
				"traces": {
					InputType: configmodels.TracesDataType,
					Receivers: []string{"jaeger"},
					Exporters: []string{kafka.TypeStr},
				},
			},
		},
		{
			storageType:    "cassandra,elasticsearch,grpc-plugin",
			zipkinHostPort: disabledHostPort,
			exporterTypes:  []string{cassandra.TypeStr, elasticsearch.TypeStr, grpcExp.TypeStr},
			pipeline: configmodels.Pipelines{
				"traces": {
					InputType: configmodels.TracesDataType,
					Receivers: []string{"jaeger"},
					Exporters: []string{cassandra.TypeStr, elasticsearch.TypeStr, grpcExp.TypeStr},
				},
			},
		},
		{
			storageType:    "cassandra",
			zipkinHostPort: ":9411",
			exporterTypes:  []string{cassandra.TypeStr},
			config:         map[string]interface{}{"resource.labels": "foo=bar"},
			pipeline: configmodels.Pipelines{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  []string{"jaeger", "zipkin"},
					Processors: []string{"resource"},
					Exporters:  []string{cassandra.TypeStr},
				},
			},
		},
		{
			storageType: "floppy",
			err:         "unknown storage type: floppy",
		},
	}
	for _, test := range tests {
		t.Run(test.storageType, func(t *testing.T) {
			v, _ := jConfig.Viperize(grpc.AddFlags)
			factories := Components(v)
			for key, val := range test.config {
				v.Set(key, val)
			}
			cfg, err := CollectorConfig(test.storageType, test.zipkinHostPort, factories)
			if test.err != "" {
				require.Nil(t, cfg)
				assert.EqualError(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, config.ValidateConfig(cfg, zap.NewNop()))

			assert.Equal(t, 1, len(cfg.Extensions))
			assert.Equal(t, 1, len(cfg.Service.Extensions))
			assert.Equal(t, "health_check", cfg.Service.Extensions[0])
			assert.Equal(t, "health_check", cfg.Extensions["health_check"].Name())
			assert.Equal(t, len(test.pipeline["traces"].Receivers), len(cfg.Receivers))
			assert.Equal(t, "jaeger", cfg.Receivers["jaeger"].Name())
			assert.Equal(t, len(test.exporterTypes), len(cfg.Exporters))

			processorMap := map[string]bool{}
			for _, p := range test.pipeline["traces"].Processors {
				processorMap[p] = true
			}
			if processorMap["resource"] {
				assert.Equal(t, len(processorMap), len(cfg.Processors))
				assert.IsType(t, &resourceprocessor.Config{}, cfg.Processors["resource"])
			}

			types := []string{}
			for _, v := range cfg.Exporters {
				types = append(types, string(v.Type()))
			}
			sort.Strings(types)
			assert.Equal(t, test.exporterTypes, types)
			assert.EqualValues(t, test.pipeline, cfg.Service.Pipelines)
		})
	}
}

func TestDefaultAgentConfig(t *testing.T) {
	tests := []struct {
		config  map[string]interface{}
		service configmodels.Service
	}{
		{
			config: map[string]interface{}{"resource.labels": "foo=bar"},
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType:  configmodels.TracesDataType,
						Receivers:  []string{"jaeger"},
						Processors: []string{"resource"},
						Exporters:  []string{"jaeger"},
					},
				},
			},
		},
		{
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType: configmodels.TracesDataType,
						Receivers: []string{"jaeger"},
						Exporters: []string{"jaeger"},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test.config), func(t *testing.T) {
			v, _ := jConfig.Viperize(grpc.AddFlags)
			for key, val := range test.config {
				v.Set(key, val)
			}
			factories := Components(v)
			cfg := AgentConfig(factories)
			require.NoError(t, config.ValidateConfig(cfg, zap.NewNop()))

			assert.Equal(t, test.service, cfg.Service)
			assert.Equal(t, 1, len(cfg.Receivers))
			assert.IsType(t, &jaegerreceiver.Config{}, cfg.Receivers["jaeger"])
			assert.Equal(t, 1, len(cfg.Exporters))
			assert.IsType(t, &jaegerexporter.Config{}, cfg.Exporters["jaeger"])
			processorMap := map[string]bool{}
			for _, p := range test.service.Pipelines["traces"].Processors {
				processorMap[p] = true
			}
			if processorMap["resource"] {
				assert.Equal(t, len(processorMap), len(cfg.Processors))
				assert.IsType(t, &resourceprocessor.Config{}, cfg.Processors["resource"])
			}
		})
	}
}

func TestDefaultIngesterConfig(t *testing.T) {
	tests := []struct {
		storageType string
		service     configmodels.Service
		err         string
	}{
		{
			storageType: "elasticsearch",
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType: configmodels.TracesDataType,
						Receivers: []string{kafkaRec.TypeStr},
						Exporters: []string{elasticsearch.TypeStr},
					},
				},
			},
		},
		{
			storageType: "elasticsearch,cassandra,grpc-plugin",
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType: configmodels.TracesDataType,
						Receivers: []string{kafkaRec.TypeStr},
						Exporters: []string{cassandra.TypeStr, elasticsearch.TypeStr, grpcExp.TypeStr},
					},
				},
			},
		},
		{
			storageType: "floppy",
			err:         "unknown storage type: floppy",
		},
	}
	for _, test := range tests {
		t.Run(test.storageType, func(t *testing.T) {
			factories := Components(viper.New())
			cfg, err := IngesterConfig(test.storageType, factories)
			if test.err != "" {
				require.Nil(t, cfg)
				assert.EqualError(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, config.ValidateConfig(cfg, zap.NewNop()))

			sort.Strings(cfg.Service.Pipelines["traces"].Exporters)
			assert.Equal(t, test.service, cfg.Service)
			assert.Equal(t, 1, len(cfg.Receivers))
			assert.IsType(t, &kafkaRec.Config{}, cfg.Receivers[kafkaRec.TypeStr])

			assert.Equal(t, len(test.service.Pipelines["traces"].Exporters), len(cfg.Exporters))
			types := []string{}
			for _, v := range cfg.Exporters {
				types = append(types, string(v.Type()))
			}
			sort.Strings(types)
			assert.Equal(t, test.service.Pipelines["traces"].Exporters, types)
		})
	}
}
