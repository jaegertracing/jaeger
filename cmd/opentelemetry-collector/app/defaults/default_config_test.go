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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/grpcplugin"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/memory"
	kafkaRec "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/kafka"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestService(t *testing.T) {
	tests := []struct {
		service     configmodels.Service
		cfg         ComponentSettings
		err         string
		viperConfig map[string]interface{}
	}{
		{
			cfg: ComponentSettings{
				ComponentType: Agent,
			},
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
		{
			viperConfig: map[string]interface{}{"resource.labels": "foo=bar"},
			cfg: ComponentSettings{
				ComponentType: Collector,
				StorageType:   "elasticsearch,kafka,memory",
			},
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType:  configmodels.TracesDataType,
						Receivers:  []string{"jaeger"},
						Processors: []string{"resource"},
						Exporters:  []string{elasticsearch.TypeStr, kafka.TypeStr, memory.TypeStr},
					},
				},
			},
		},
		{
			cfg: ComponentSettings{
				ComponentType: Ingester,
				StorageType:   "elasticsearch",
			},
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
			cfg: ComponentSettings{
				ComponentType: Ingester,
				StorageType:   "cassandra,elasticsearch,grpc-plugin",
			},
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType: configmodels.TracesDataType,
						Receivers: []string{kafkaRec.TypeStr},
						Exporters: []string{cassandra.TypeStr, elasticsearch.TypeStr, grpcplugin.TypeStr},
					},
				},
			},
		},
		{
			cfg: ComponentSettings{
				ComponentType:  AllInOne,
				StorageType:    "elasticsearch",
				ZipkinHostPort: "localhost:9411",
			},
			service: configmodels.Service{
				Extensions: []string{"health_check"},
				Pipelines: configmodels.Pipelines{
					"traces": &configmodels.Pipeline{
						InputType: configmodels.TracesDataType,
						Receivers: []string{"jaeger", "zipkin"},
						Exporters: []string{elasticsearch.TypeStr},
					},
				},
			},
		},
		{
			cfg: ComponentSettings{
				ComponentType: Collector,
				StorageType:   "floppy",
			},
			err: "unknown storage type: floppy",
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v:%v", test.cfg.ComponentType, test.cfg.StorageType), func(t *testing.T) {
			v, _ := jConfig.Viperize(app.AddComponentFlags)
			for key, val := range test.viperConfig {
				v.Set(key, val)
			}
			factories := Components(v)
			test.cfg.Factories = factories
			cfg, err := test.cfg.CreateDefaultConfig()
			if test.err != "" {
				require.Nil(t, cfg)
				assert.Contains(t, err.Error(), test.err)
				return
			}
			sort.Strings(test.service.Pipelines["traces"].Exporters)
			sort.Strings(cfg.Service.Pipelines["traces"].Exporters)
			sort.Strings(test.service.Pipelines["traces"].Receivers)
			sort.Strings(cfg.Service.Pipelines["traces"].Receivers)
			require.NoError(t, err)
			require.NoError(t, config.ValidateConfig(cfg, zap.NewNop()))
			assert.Equal(t, test.service, cfg.Service)

			assert.Equal(t, len(test.service.Pipelines["traces"].Exporters), len(cfg.Exporters))
			types := []string{}
			for _, e := range cfg.Exporters {
				types = append(types, string(e.Type()))
			}
			sort.Strings(types)
			assert.Equal(t, test.service.Pipelines["traces"].Exporters, types)

			assert.Equal(t, len(test.service.Pipelines["traces"].Receivers), len(cfg.Receivers))
			types = []string{}
			for _, r := range cfg.Receivers {
				types = append(types, string(r.Type()))
			}
			sort.Strings(types)
			assert.Equal(t, test.service.Pipelines["traces"].Receivers, types)
		})
	}
}
