// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package translator

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/olivere/elastic"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

const (
	movFnAggName    = "results" // movFnAggName is the name of the moving function aggregation in Elasticsearch, same as in reader.go
	dateHistAggName = "results_buckets"
)

// Translator converts Elasticsearch aggregations to Jaeger's metrics model.
type Translator struct{}

// New returns a new Translator.
func New() Translator {
	return Translator{}
}

// ToMetricsFamily converts Elasticsearch aggregations to Jaeger's MetricFamily.
func (d Translator) ToMetricsFamily(name, description string, labels []*metrics.Label, result *elastic.SearchResult) (*metrics.MetricFamily, error) {
	domainMetrics, err := d.toDomainMetrics(labels, result)
	if err != nil {
		return nil, fmt.Errorf("failed to convert aggregations to metrics: %w", err)
	}

	return &metrics.MetricFamily{
		Name:    name,
		Type:    metrics.MetricType_GAUGE,
		Help:    description,
		Metrics: domainMetrics,
	}, nil
}

// toDomainMetrics converts Elasticsearch aggregations to Jaeger metrics.
func (d Translator) toDomainMetrics(labels []*metrics.Label, result *elastic.SearchResult) ([]*metrics.Metric, error) {
	buckets, err := d.extractBuckets(result)
	if err != nil {
		return nil, err
	}

	return []*metrics.Metric{
		{
			Labels:       labels,
			MetricPoints: d.toDomainMetricPoints(buckets),
		},
	}, nil
}

// extractBuckets retrieves date histogram buckets from Elasticsearch results.
func (d Translator) extractBuckets(result *elastic.SearchResult) ([]*elastic.AggregationBucketHistogramItem, error) {
	agg, found := result.Aggregations.DateHistogram(dateHistAggName)
	if !found {
		return nil, errors.New("results_buckets aggregation not found")
	}
	return agg.Buckets, nil
}

// toDomainMetricPoints converts Elasticsearch buckets to Jaeger metric points.
func (d Translator) toDomainMetricPoints(buckets []*elastic.AggregationBucketHistogramItem) []*metrics.MetricPoint {
	metricPoints := make([]*metrics.MetricPoint, 0, len(buckets))
	for _, bucket := range buckets {
		mp := d.toDomainMetricPoint(bucket)
		if mp != nil {
			metricPoints = append(metricPoints, mp)
		}
	}

	return metricPoints
}

// toDomainMetricPoint converts a single Elasticsearch bucket to a Jaeger metric point.
func (d Translator) toDomainMetricPoint(bucket *elastic.AggregationBucketHistogramItem) *metrics.MetricPoint {
	rateAgg, found := bucket.Aggregations.MovFn(movFnAggName)
	if !found || rateAgg.Value == nil {
		return nil
	}

	timestamp := d.toDomainTimestamp(int64(bucket.Key))
	if timestamp == nil {
		return nil
	}

	return &metrics.MetricPoint{
		Value:     d.toDomainMetricPointValue(*rateAgg.Value),
		Timestamp: timestamp,
	}
}

// toDomainTimestamp converts milliseconds since epoch to protobuf Timestamp.
func (d Translator) toDomainTimestamp(millis int64) *types.Timestamp {
	timestamp := time.Unix(0, millis*int64(time.Millisecond))
	protoTimestamp, err := types.TimestampProto(timestamp)
	if err != nil {
		return nil
	}
	return protoTimestamp
}

// toDomainMetricPointValue converts a float64 value to Jaeger's gauge metric point.
func (d Translator) toDomainMetricPointValue(value float64) *metrics.MetricPoint_GaugeValue {
	// Round to 2 decimal places
	roundedValue := math.Round(value*100) / 100
	return &metrics.MetricPoint_GaugeValue{
		GaugeValue: &metrics.GaugeValue{
			Value: &metrics.GaugeValue_DoubleValue{DoubleValue: roundedValue},
		},
	}
}
