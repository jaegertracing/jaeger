// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/shared/processor"
)

// ScaleAndRoundLatencies processes raw latency metrics by applying scaling and rounding.
func ScaleAndRoundLatencies(mf *metrics.MetricFamily) *metrics.MetricFamily {
	// Delegates to shared processor package. Will be removed in a future PR.
	return processor.ScaleAndRoundLatencies(mf)
}

// CalculateCallRates processes raw call rate metrics by calculating rates and trimming to time range.
func CalculateCallRates(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters, timeRange TimeRange) *metrics.MetricFamily {
	// Delegates to shared processor package. Will be removed in a future PR.
	return processor.CalculateCallRates(mf, params, toSharedTimeRange(timeRange))
}

// CalculateErrorRates processes error rate metrics by computing error rates from errors and calls.
func CalculateErrorRates(rawErrors, calls *metrics.MetricFamily, params metricstore.BaseQueryParameters, timeRange TimeRange) *metrics.MetricFamily {
	// Delegates to shared processor package. Will be removed in a future PR.
	return processor.CalculateErrorRates(rawErrors, calls, params, toSharedTimeRange(timeRange))
}

// toSharedTimeRange converts metricstore TimeRange to shared processor TimeRange.
func toSharedTimeRange(tr TimeRange) processor.TimeRange {
	return processor.TimeRange{
		StartTimeMillis:         tr.startTimeMillis,
		EndTimeMillis:           tr.endTimeMillis,
		ExtendedStartTimeMillis: tr.extendedStartTimeMillis,
	}
}

// Unexported helper functions below delegate to shared package for testing purposes.
// These will be removed in a future PR once tests are migrated.

func calculateErrorRateValue(errorPoint, callPoint *metrics.MetricPoint) float64 {
	return processor.CalculateErrorRateValue(errorPoint, callPoint)
}

func zeroValue(points []*metrics.MetricPoint) []*metrics.MetricPoint {
	return processor.ZeroValue(points)
}

func trimMetricPointsBefore(mf *metrics.MetricFamily, startMillis int64) *metrics.MetricFamily {
	return processor.TrimMetricPointsBefore(mf, startMillis)
}

func scaleToMillisAndRound(metric *metrics.Metric, window []*metrics.MetricPoint) float64 {
	return processor.ScaleToMillisAndRound(metric, window)
}
