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

package defaultcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/badgerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/cassandraexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/grpcpluginexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/jaegerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/kafkaexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/memoryexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/jaegerreceiver"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/kafkareceiver"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/receiver/zipkinreceiver"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestComponents(t *testing.T) {
	v, _ := jConfig.Viperize(
		cassandraexporter.DefaultOptions().AddFlags,
		elasticsearchexporter.DefaultOptions().AddFlags,
	)
	factories := Components(v)
	assert.IsType(t, &kafkaexporter.Factory{}, factories.Exporters[kafkaexporter.TypeStr])
	assert.IsType(t, &cassandraexporter.Factory{}, factories.Exporters[cassandraexporter.TypeStr])
	assert.IsType(t, &elasticsearchexporter.Factory{}, factories.Exporters[elasticsearchexporter.TypeStr])
	assert.IsType(t, &grpcpluginexporter.Factory{}, factories.Exporters[grpcpluginexporter.TypeStr])
	assert.IsType(t, &memoryexporter.Factory{}, factories.Exporters[memoryexporter.TypeStr])
	assert.IsType(t, &badgerexporter.Factory{}, factories.Exporters[badgerexporter.TypeStr])
	assert.IsType(t, &jaegerreceiver.Factory{}, factories.Receivers["jaeger"])
	assert.IsType(t, &jaegerexporter.Factory{}, factories.Exporters["jaeger"])
	assert.IsType(t, &kafkareceiver.Factory{}, factories.Receivers[kafkareceiver.TypeStr])
	assert.IsType(t, &zipkinreceiver.Factory{}, factories.Receivers["zipkin"])

	cassandraFactory := factories.Exporters[cassandraexporter.TypeStr]
	cc := cassandraFactory.CreateDefaultConfig().(*cassandraexporter.Config)
	assert.Equal(t, []string{"127.0.0.1"}, cc.Options.GetPrimary().Servers)

	esFactory := factories.Exporters[elasticsearchexporter.TypeStr]
	ec := esFactory.CreateDefaultConfig().(*elasticsearchexporter.Config)
	assert.Equal(t, []string{"http://127.0.0.1:9200"}, ec.GetPrimary().Servers)

	grpcFactory := factories.Exporters[grpcpluginexporter.TypeStr]
	gc := grpcFactory.CreateDefaultConfig().(*grpcpluginexporter.Config)
	assert.Equal(t, "", gc.Configuration.PluginBinary)

	badgerFactory := factories.Exporters[badgerexporter.TypeStr]
	bc := badgerFactory.CreateDefaultConfig().(*badgerexporter.Config)
	assert.Equal(t, "", bc.GetPrimary().ValueDirectory)
}
