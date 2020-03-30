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

	"github.com/magiconair/properties/assert"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestComponents(t *testing.T) {
	v, _ := jConfig.Viperize(kafka.DefaultOptions().AddFlags, cassandra.DefaultOptions().AddFlags, elasticsearch.DefaultOptions().AddFlags)
	factories := Components(v)
	assert.Equal(t, "jaeger_kafka", factories.Exporters[kafka.TypeStr].Type())
	assert.Equal(t, "jaeger_cassandra", factories.Exporters[cassandra.TypeStr].Type())
	assert.Equal(t, "jaeger_elasticsearch", factories.Exporters[elasticsearch.TypeStr].Type())

	kafkaFactory := factories.Exporters[kafka.TypeStr]
	kc := kafkaFactory.CreateDefaultConfig().(*kafka.Config)
	assert.Equal(t, []string{"127.0.0.1:9092"}, kc.Config.Brokers)

	cassandraFactory := factories.Exporters[cassandra.TypeStr]
	cc := cassandraFactory.CreateDefaultConfig().(*cassandra.Config)
	assert.Equal(t, []string{"127.0.0.1"}, cc.Options.GetPrimary().Servers)
	esFactory := factories.Exporters[elasticsearch.TypeStr]
	ec := esFactory.CreateDefaultConfig().(*elasticsearch.Config)
	assert.Equal(t, []string{"http://127.0.0.1:9200"}, ec.GetPrimary().Servers)
}
