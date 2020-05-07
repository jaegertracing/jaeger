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

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/cassandra"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/elasticsearch"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/jaegerexporter"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter/kafka"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/jaegerreceiver"
	kafkaRec "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/receiver/kafka"
	jConfig "github.com/jaegertracing/jaeger/pkg/config"
)

func TestComponents(t *testing.T) {
	v, _ := jConfig.Viperize(
		kafka.DefaultOptions().AddFlags,
		cassandra.DefaultOptions().AddFlags,
		elasticsearch.DefaultOptions().AddFlags,
	)
	factories := Components(v)
	assert.IsType(t, &kafka.Factory{}, factories.Exporters[kafka.TypeStr])
	assert.IsType(t, &cassandra.Factory{}, factories.Exporters[cassandra.TypeStr])
	assert.IsType(t, &elasticsearch.Factory{}, factories.Exporters[elasticsearch.TypeStr])
	assert.IsType(t, &grpc.Factory{}, factories.Exporters[grpc.TypeStr])
	assert.IsType(t, &jaegerreceiver.Factory{}, factories.Receivers["jaeger"])
	assert.IsType(t, &jaegerexporter.Factory{}, factories.Exporters["jaeger"])
	assert.IsType(t, &kafkaRec.Factory{}, factories.Receivers[kafkaRec.TypeStr])

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
