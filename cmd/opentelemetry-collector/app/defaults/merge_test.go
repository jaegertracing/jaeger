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
	"testing"

	"github.com/open-telemetry/opentelemetry-collector/config"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector/processor/batchprocessor"
	"github.com/open-telemetry/opentelemetry-collector/receiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector/receiver/zipkinreceiver"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestMergeConfigs_nil(t *testing.T) {
	cfg := &configmodels.Config{
		Receivers: configmodels.Receivers{
			"jaeger": &jaegerreceiver.Config{
				RemoteSampling: &jaegerreceiver.RemoteSamplingConfig{StrategyFile: "file.json"},
			},
		},
	}
	err := MergeConfigs(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, cfg, cfg)
}

func TestMergeConfigs(t *testing.T) {
	cfg := &configmodels.Config{
		Receivers: configmodels.Receivers{
			"jaeger": &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"grpc":           {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "def"}},
					"thrift_compact": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "def"}},
				},
			},
		},
		Processors: configmodels.Processors{
			"batch": &batchprocessor.Config{
				SendBatchSize: uint32(160),
			},
		},
		Service: configmodels.Service{
			Extensions: []string{"def", "def2"},
			Pipelines: configmodels.Pipelines{
				"traces": &configmodels.Pipeline{
					Receivers:  []string{"jaeger"},
					Processors: []string{"batch"},
				},
			},
		},
	}
	overrideCfg := &configmodels.Config{
		Receivers: configmodels.Receivers{
			"jaeger": &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"grpc": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "master_jaeger_url"}},
				},
			},
			"zipkin": &zipkinreceiver.Config{
				ReceiverSettings: configmodels.ReceiverSettings{
					Endpoint: "master_zipkin_url",
				},
			},
		},
		Processors: configmodels.Processors{
			"attributes": &attributesprocessor.Config{
				Actions: []attributesprocessor.ActionKeyValue{{Key: "foo"}},
			},
		},
		Service: configmodels.Service{
			Extensions: []string{"def", "master1", "master2"},
			Pipelines: configmodels.Pipelines{
				"traces": &configmodels.Pipeline{
					Receivers:  []string{"jaeger", "zipkin"},
					Processors: []string{"attributes"},
				},
				"traces/2": &configmodels.Pipeline{
					Processors: []string{"example"},
				},
			},
		},
	}
	expected := &configmodels.Config{
		Receivers: configmodels.Receivers{
			"jaeger": &jaegerreceiver.Config{
				Protocols: map[string]*receiver.SecureReceiverSettings{
					"grpc":           {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "master_jaeger_url"}},
					"thrift_compact": {ReceiverSettings: configmodels.ReceiverSettings{Endpoint: "def"}},
				},
			},
			"zipkin": &zipkinreceiver.Config{
				ReceiverSettings: configmodels.ReceiverSettings{
					Endpoint: "master_zipkin_url",
				},
			},
		},
		Processors: configmodels.Processors{
			"batch": &batchprocessor.Config{
				SendBatchSize: uint32(160),
			},
			"attributes": &attributesprocessor.Config{
				Actions: []attributesprocessor.ActionKeyValue{{Key: "foo"}},
			},
		},
		Service: configmodels.Service{
			Extensions: []string{"def", "master1", "master2"},
			Pipelines: configmodels.Pipelines{
				"traces": &configmodels.Pipeline{
					Receivers:  []string{"jaeger", "zipkin"},
					Processors: []string{"attributes"},
				},
				"traces/2": &configmodels.Pipeline{
					Processors: []string{"example"},
				},
			},
		},
	}
	err := MergeConfigs(cfg, overrideCfg)
	require.NoError(t, err)
	assert.Equal(t, expected, cfg)
}

func TestMergeConfigFiles(t *testing.T) {
	testFiles := []string{"emptyoverride", "addprocessor", "multiplecomponents"}
	v, _ := jConfig.Viperize(elasticsearch.DefaultOptions().AddFlags)
	cmpts := Components(v)
	for _, f := range testFiles {
		t.Run(f, func(t *testing.T) {
			cfg, err := loadConfig(cmpts, fmt.Sprintf("testdata/%s.yaml", f))
			require.NoError(t, err)
			override, err := loadConfig(cmpts, fmt.Sprintf("testdata/%s-override.yaml", f))
			require.NoError(t, err)
			merged, err := loadConfig(cmpts, fmt.Sprintf("testdata/%s-merged.yaml", f))
			require.NoError(t, err)
			err = MergeConfigs(cfg, override)
			require.NoError(t, err)
			assert.Equal(t, merged, cfg)
		})
	}
}

func loadConfig(factories config.Factories, file string) (*configmodels.Config, error) {
	v := viper.New()
	v.SetConfigFile(file)
	err := v.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading config file %q: %v", file, err)
	}
	return config.Load(v, factories)
}
