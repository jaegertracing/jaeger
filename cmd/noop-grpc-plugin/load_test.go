package main

import (
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
	"testing"
	"time"
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
