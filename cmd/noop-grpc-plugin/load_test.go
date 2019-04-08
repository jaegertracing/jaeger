// Copyright (c) 2019 The Jaeger Authors.
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

package main

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
)

var logger = zap.NewNop()

func BenchmarkNoopSpanWriter(b *testing.B) {
	s := &noopStore{}

	for n := 0; n < b.N; n++ {
		s.WriteSpan(&model.Span{
			TraceID:model.NewTraceID(1, 2),
			SpanID:model.NewSpanID(1),
			OperationName: "test",
			StartTime: time.Now(),
			Duration: 1 * time.Second,
			Process: model.NewProcess("process", []model.KeyValue{

			}),
			ProcessID: "process_id",
			Tags: []model.KeyValue{
				{
					Key:"test",
					VStr: "",
				},
			},
		})
	}
}

func BenchmarkGRPCNoopSpanWriter(b *testing.B) {
	v := viper.New()

	v.Set("grpc-storage-plugin.binary", "noop-grpc-plugin")

	f := grpc.NewFactory()
	f.InitFromViper(v)

	metricsFactory := prometheus.New()

	f.Initialize( metricsFactory, logger)

	sw, err := f.CreateSpanWriter()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		sw.WriteSpan(&model.Span{
			TraceID:model.NewTraceID(1, 2),
			SpanID:model.NewSpanID(1),
			OperationName: "test",
			StartTime: time.Now(),
			Duration: 1 * time.Second,
			Process: model.NewProcess("process", []model.KeyValue{

			}),
			ProcessID: "process_id",
			Tags: []model.KeyValue{
				{
					Key:"test",
					VStr: "",
				},
			},
		})
	}
}
