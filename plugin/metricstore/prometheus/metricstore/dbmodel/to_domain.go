// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/prometheus/common/model"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

// Translator translates Prometheus's metrics model to Jaeger's.
type Translator struct {
	labelMap map[string]string
}

// New returns a new Translator.
func New(spanNameLabel string) Translator {
	return Translator{
		// "operation" is the label name that Jaeger UI expects.
		labelMap: map[string]string{spanNameLabel: "operation"},
	}
}

// ToDomainMetricsFamily converts Prometheus' representation of metrics query results to Jaeger's.
func (d Translator) ToDomainMetricsFamily(name, description string, mv model.Value) (*metrics.MetricFamily, error) {
	if mv.Type() != model.ValMatrix {
		return &metrics.MetricFamily{}, fmt.Errorf("unexpected metrics ValueType: %s", mv.Type())
	}
	return &metrics.MetricFamily{
		Name:    name,
		Type:    metrics.MetricType_GAUGE,
		Help:    description,
		Metrics: d.toDomainMetrics(mv.(model.Matrix)),
	}, nil
}

// toDomainMetrics converts Prometheus' representation of metrics to Jaeger's.
func (d Translator) toDomainMetrics(matrix model.Matrix) []*metrics.Metric {
	ms := make([]*metrics.Metric, matrix.Len())
	for i, ss := range matrix {
		ms[i] = &metrics.Metric{
			Labels:       d.toDomainLabels(ss.Metric),
			MetricPoints: toDomainMetricPoints(ss.Values),
		}
	}
	return ms
}

// toDomainLabels converts Prometheus' representation of metric labels to Jaeger's.
func (d Translator) toDomainLabels(promLabels model.Metric) []*metrics.Label {
	labels := make([]*metrics.Label, len(promLabels))
	j := 0
	for k, v := range promLabels {
		labelName := string(k)
		if newLabel, ok := d.labelMap[labelName]; ok {
			labelName = newLabel
		}
		labels[j] = &metrics.Label{Name: labelName, Value: string(v)}
		j++
	}
	return labels
}

// toDomainMetricPoints convert's Prometheus' representation of metrics data points to Jaeger's.
func toDomainMetricPoints(promDps []model.SamplePair) []*metrics.MetricPoint {
	domainMps := make([]*metrics.MetricPoint, len(promDps))
	for i, promDp := range promDps {
		mp := &metrics.MetricPoint{
			Timestamp: toDomainTimestamp(promDp.Timestamp),
			Value:     toDomainMetricPointValue(promDp.Value),
		}
		domainMps[i] = mp
	}
	return domainMps
}

// toDomainTimestamp converts Prometheus' representation of timestamps to Jaeger's.
func toDomainTimestamp(timeMs model.Time) *types.Timestamp {
	return &types.Timestamp{
		Seconds: int64(timeMs / 1000),
		//nolint: gosec // G115
		Nanos: int32((timeMs % 1000) * 1_000_000),
	}
}

// toDomainMetricPointValue converts Prometheus' representation of a double gauge value to Jaeger's.
// The gauge metric type is used because latency, call and error rates metrics do not consist of monotonically
// increasing values; rather, they are a series of any positive floating number which can fluctuate in any
// direction over time.
func toDomainMetricPointValue(promVal model.SampleValue) *metrics.MetricPoint_GaugeValue {
	return &metrics.MetricPoint_GaugeValue{
		GaugeValue: &metrics.GaugeValue{
			Value: &metrics.GaugeValue_DoubleValue{DoubleValue: float64(promVal)},
		},
	}
}
