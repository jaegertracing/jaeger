package metricstore

import (
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

// metricsRow represents a single row from the SPM aggregation queries.
type metricsRow struct {
	Timestamp   time.Time
	ServiceName string
	Operation   string
	Value       float64
}

func (mr *metricsRow) toMetricPoint() *metrics.MetricPoint {
	return &metrics.MetricPoint{
		Timestamp: &types.Timestamp{
			Seconds: mr.Timestamp.Unix(),
			Nanos:   int32(mr.Timestamp.Nanosecond()),
		},
		Value: &metrics.MetricPoint_GaugeValue{
			GaugeValue: &metrics.GaugeValue{
				Value: &metrics.GaugeValue_DoubleValue{DoubleValue: mr.Value},
			},
		},
	}
}
