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
	"testing"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
)

func TestDefaultConfig(t *testing.T) {
	factories := Components(viper.New())
	tests := []struct {
		storageType   string
		exporterTypes []string
		pipeline      map[string]*configmodels.Pipeline
	}{
		{
			storageType:   "elasticsearch",
			exporterTypes: []string{elasticsearch.TypeStr},
			pipeline: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  []string{"jaeger"},
					Exporters:  []string{elasticsearch.TypeStr},
					Processors: []string{"batch"},
				},
			},
		},
		{
			storageType:   "cassandra",
			exporterTypes: []string{cassandra.TypeStr},
			pipeline: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  []string{"jaeger"},
					Exporters:  []string{cassandra.TypeStr},
					Processors: []string{"batch"},
				},
			},
		},
		{
			storageType:   "cassandra,elasticsearch",
			exporterTypes: []string{cassandra.TypeStr, elasticsearch.TypeStr},
			pipeline: map[string]*configmodels.Pipeline{
				"traces": {
					InputType:  configmodels.TracesDataType,
					Receivers:  []string{"jaeger"},
					Exporters:  []string{cassandra.TypeStr, elasticsearch.TypeStr},
					Processors: []string{"batch"},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.storageType, func(t *testing.T) {
			cfg := DefaultConfig(test.storageType, factories)
			require.NoError(t, config.ValidateConfig(cfg, zap.NewNop()))

			assert.Equal(t, 1, len(cfg.Receivers))
			assert.Equal(t, "jaeger", cfg.Receivers["jaeger"].Name())
			assert.Equal(t, 1, len(cfg.Processors))
			assert.Equal(t, "batch", cfg.Processors["batch"].Name())
			assert.Equal(t, len(test.exporterTypes), len(cfg.Exporters))

			types := []string{}
			for _, v := range cfg.Exporters {
				types = append(types, v.Type())
			}
			assert.Equal(t, test.exporterTypes, types)
			assert.EqualValues(t, test.pipeline, cfg.Service.Pipelines)
		})
	}

}
