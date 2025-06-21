// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"fmt"
	"math"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/olivere/elastic"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

// Translator converts Elasticsearch aggregations to Jaeger's metrics model.
type Translator struct{}

// ToMetricsFamily converts Elasticsearch aggregations to Jaeger's MetricFamily.
func (d Translator) ToMetricsFamily(m MetricsQueryParams, result *elastic.SearchResult) (*metrics.MetricFamily, error) {
	domainMetrics, err := d.toDomainMetrics(m, result)
	if err != nil {
		return nil, fmt.Errorf("failed to convert aggregations to metrics: %w", err)
	}

	return &metrics.MetricFamily{
		Name:    m.metricName,
		Type:    metrics.MetricType_GAUGE,
		Help:    m.metricDesc,
		Metrics: domainMetrics,
	}, nil
}

// toDomainMetrics converts Elasticsearch aggregations to Jaeger metrics.
func (d Translator) toDomainMetrics(m MetricsQueryParams, result *elastic.SearchResult) ([]*metrics.Metric, error) {
	labels := buildServiceLabels(m.ServiceNames)

	if !m.GroupByOperation {
		buckets, err := d.extractBuckets(result)
		if err != nil {
			return nil, err
		}
		return []*metrics.Metric{
			{
				Labels:       labels,
				MetricPoints: d.toDomainMetricPoints(m, buckets),
			},
		}, nil
	}

	// Handle grouped results when groupByOp is true
	agg, found := result.Aggregations.Terms(aggName)
	if !found {
		return nil, fmt.Errorf("%s aggregation not found", aggName)
	}

	var metricsData []*metrics.Metric
	for _, bucket := range agg.Buckets {
		metric, err := d.processOperationBucket(m, bucket, labels)
		if err != nil {
			return nil, fmt.Errorf("failed to process bucket: %w", err)
		}
		metricsData = append(metricsData, metric)
	}

	return metricsData, nil
}

func buildServiceLabels(serviceNames []string) []*metrics.Label {
	labels := make([]*metrics.Label, len(serviceNames))
	for i, name := range serviceNames {
		labels[i] = &metrics.Label{Name: "service_name", Value: name}
	}
	return labels
}

func (d Translator) processOperationBucket(m MetricsQueryParams, bucket *elastic.AggregationBucketKeyItem, baseLabels []*metrics.Label) (*metrics.Metric, error) {
	key, ok := bucket.Key.(string)
	if !ok {
		return nil, fmt.Errorf("bucket key is not a string: %v", bucket.Key)
	}

	// Extract nested date_histogram buckets
	dateHistAgg, found := bucket.Aggregations.DateHistogram("date_histogram")
	if !found {
		return nil, fmt.Errorf("date_histogram aggregation not found in bucket %q", key)
	}

	// Combine base labels with operation label
	labels := append(baseLabels, d.toDomainLabels(key)...)

	return &metrics.Metric{
		Labels:       labels,
		MetricPoints: d.toDomainMetricPoints(m, dateHistAgg.Buckets),
	}, nil
}

// toDomainLabels converts the bucket key to Jaeger metric labels.
func (Translator) toDomainLabels(key string) []*metrics.Label {
	return []*metrics.Label{
		{
			Name:  "operation",
			Value: key,
		},
	}
}

// extractBuckets retrieves date histogram buckets from Elasticsearch results.
func (Translator) extractBuckets(result *elastic.SearchResult) ([]*elastic.AggregationBucketHistogramItem, error) {
	agg, found := result.Aggregations.DateHistogram(aggName)
	if !found {
		return nil, fmt.Errorf("%s aggregation not found", aggName)
	}
	return agg.Buckets, nil
}

// toDomainMetricPoints converts Elasticsearch buckets to Jaeger metric points.
func (d Translator) toDomainMetricPoints(m MetricsQueryParams, buckets []*elastic.AggregationBucketHistogramItem) []*metrics.MetricPoint {
	result := translateBucketsToPointsArray(buckets)
	processOutput := m.processMetricsFunc(result)

	metricPoints := make([]*metrics.MetricPoint, 0, len(processOutput))
	for _, pair := range processOutput {
		mp := d.toDomainMetricPoint(pair)
		if mp != nil {
			metricPoints = append(metricPoints, mp)
		}
	}

	return metricPoints
}

func translateBucketsToPointsArray(buckets []*elastic.AggregationBucketHistogramItem) []*Pair {
	var points []*Pair

	for _, bucket := range buckets {
		aggMap, ok := bucket.Aggregations.CumulativeSum(culmuAggName)
		if !ok {
			return nil
		}
		value := math.NaN()
		if aggMap != nil && aggMap.Value != nil {
			value = *aggMap.Value
		}
		points = append(points, &Pair{
			TimeStamp: int64(bucket.Key),
			Value:     value,
		})
	}
	return points
}

// toDomainMetricPoint converts a single Pair to a Jaeger metric point.
func (d Translator) toDomainMetricPoint(pair *Pair) *metrics.MetricPoint {
	timestamp := d.toDomainTimestamp(pair.TimeStamp)
	if timestamp == nil {
		return nil
	}

	return &metrics.MetricPoint{
		Value:     d.toDomainMetricPointValue(pair.Value),
		Timestamp: timestamp,
	}
}

// toDomainTimestamp converts milliseconds since epoch to protobuf Timestamp.
func (Translator) toDomainTimestamp(millis int64) *types.Timestamp {
	timestamp := time.Unix(0, millis*int64(time.Millisecond))
	protoTimestamp, _ := types.TimestampProto(timestamp)
	return protoTimestamp
}

// toDomainMetricPointValue converts a float64 value to Jaeger's gauge metric point.
func (Translator) toDomainMetricPointValue(value float64) *metrics.MetricPoint_GaugeValue {
	// Round to 2 decimal places
	roundedValue := math.Round(value*100) / 100
	return &metrics.MetricPoint_GaugeValue{
		GaugeValue: &metrics.GaugeValue{
			Value: &metrics.GaugeValue_DoubleValue{DoubleValue: roundedValue},
		},
	}
}
