// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

// ScaleAndRoundLatencies processes raw latency metrics by applying scaling and rounding.
func ScaleAndRoundLatencies(mf *metrics.MetricFamily) *metrics.MetricFamily {
	const lookback = 1 // only current value
	return applySlidingWindow(mf, lookback, scaleToMillisAndRound)
}

// CalculateCallRates processes raw call rate metrics by calculating rates and trimming to time range.
func CalculateCallRates(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters, timeRange TimeRange) *metrics.MetricFamily {
	processed := calcCallRate(mf, params)
	return trimMetricPointsBefore(processed, timeRange.startTimeMillis)
}

// CalculateErrorRates processes error rate metrics by computing error rates from errors and calls.
func CalculateErrorRates(rawErrors, calls *metrics.MetricFamily, params metricstore.BaseQueryParameters, timeRange TimeRange) *metrics.MetricFamily {
	processedErrors := CalculateCallRates(rawErrors, params, timeRange)
	return calcErrorRates(processedErrors, calls)
}

// calcErrorRates computes error rates by dividing error metrics by call metrics.
func calcErrorRates(errorMetrics, callMetrics *metrics.MetricFamily) *metrics.MetricFamily {
	result := &metrics.MetricFamily{
		Name:    errorMetrics.Name,
		Type:    metrics.MetricType_GAUGE,
		Help:    errorMetrics.Help,
		Metrics: make([]*metrics.Metric, 0, len(errorMetrics.Metrics)),
	}

	// Build a lookup map for error metrics by their labels.
	errorMetricsByLabels := make(map[string]*metrics.Metric)
	for _, errorMetric := range errorMetrics.Metrics {
		labelKey := getLabelKey(errorMetric.Labels)
		errorMetricsByLabels[labelKey] = errorMetric
	}

	for _, callMetric := range callMetrics.Metrics {
		labelKey := getLabelKey(callMetric.Labels)
		errorMetric, exists := errorMetricsByLabels[labelKey]
		if !exists {
			// If do not exist, it means that no data for error span, returning 0 error rate.
			metricPoints := zeroValue(callMetric.MetricPoints)
			result.Metrics = append(result.Metrics, &metrics.Metric{
				Labels:       callMetric.Labels,
				MetricPoints: metricPoints,
			})
			continue
		}

		metricPoints := calculateErrorRatePoints(errorMetric.MetricPoints, callMetric.MetricPoints)
		result.Metrics = append(result.Metrics, &metrics.Metric{
			Labels:       callMetric.Labels,
			MetricPoints: metricPoints,
		})
	}

	return result
}

// zeroValue creates metric points with zero values matching the timestamps of the input points
func zeroValue(points []*metrics.MetricPoint) []*metrics.MetricPoint {
	zeroPoints := make([]*metrics.MetricPoint, 0, len(points))

	for _, point := range points {
		value := math.NaN()
		// Only append 0 for call_points value is not NaN
		if !math.IsNaN(point.GetGaugeValue().GetDoubleValue()) {
			value = 0.0
		}
		zeroPoints = append(zeroPoints, &metrics.MetricPoint{
			Timestamp: point.Timestamp,
			Value:     toDomainMetricPointValue(value),
		})
	}

	return zeroPoints
}

// calculateErrorRatePoints computes error rates for corresponding metric points.
func calculateErrorRatePoints(errorPoints, callPoints []*metrics.MetricPoint) []*metrics.MetricPoint {
	metricPoints := make([]*metrics.MetricPoint, 0, len(errorPoints))

	// Build a lookup map for call points by timestamp.
	callPointsByTime := make(map[int64]*metrics.MetricPoint)
	for _, callPoint := range callPoints {
		key, _ := timestampToKey(callPoint.Timestamp)
		callPointsByTime[key] = callPoint
	}

	for _, errorPoint := range errorPoints {
		key, _ := timestampToKey(errorPoint.Timestamp)
		callPoint, exists := callPointsByTime[key]
		if !exists {
			continue // Skip if no matching timestamp.
		}

		value := calculateErrorRateValue(errorPoint, callPoint)
		metricPoints = append(metricPoints, &metrics.MetricPoint{
			Timestamp: errorPoint.Timestamp,
			Value:     toDomainMetricPointValue(value),
		})
	}

	return metricPoints
}

// Helper function to generate a unique key for labels.
func getLabelKey(labels []*metrics.Label) string {
	// Defensive copying
	labelsCopy := make([]*metrics.Label, len(labels))
	copy(labelsCopy, labels)

	// Sort by Name first, then by Value
	sort.Slice(labelsCopy, func(i, j int) bool {
		if labelsCopy[i].Name == labelsCopy[j].Name {
			return labelsCopy[i].Value < labelsCopy[j].Value
		}
		return labelsCopy[i].Name < labelsCopy[j].Name
	})

	keys := make([]string, len(labelsCopy))
	for i, label := range labelsCopy {
		keys[i] = fmt.Sprintf("%s=%s", label.Name, label.Value)
	}
	return strings.Join(keys, ",")
}

// Function to convert types.Timestamp to int64 (Unix nanoseconds)
func timestampToKey(ts *types.Timestamp) (int64, error) {
	t, err := types.TimestampFromProto(ts)
	if err != nil {
		return 0, err
	}
	return t.UnixNano(), nil
}

// calculateErrorRateValue computes the error rate for a single point.
func calculateErrorRateValue(errorPoint, callPoint *metrics.MetricPoint) float64 {
	errorValue := errorPoint.GetGaugeValue().GetDoubleValue()
	callValue := callPoint.GetGaugeValue().GetDoubleValue()

	if callValue == 0 {
		return 0.0
	}

	if math.IsNaN(errorValue) && !math.IsNaN(callValue) {
		return 0.0
	}

	if math.IsNaN(errorValue) || math.IsNaN(callValue) {
		return math.NaN()
	}
	return errorValue / callValue
}

// calcCallRate defines the rate calculation logic and pass in applySlidingWindow.
func calcCallRate(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters) *metrics.MetricFamily {
	lookback := int(math.Ceil(float64(params.RatePer.Milliseconds()) / float64(params.Step.Milliseconds())))
	// Ensure lookback >= 1
	lookback = int(math.Max(float64(lookback), 1))

	windowSizeSeconds := float64(lookback) * params.Step.Seconds()

	lastNonNaNMap := make(map[string]float64)

	// rateCalculator is a closure that captures 'lookback' and 'windowSizeSeconds'.
	// It implements the specific logic for calculating the rate.
	rateCalculator := func(metric *metrics.Metric, window []*metrics.MetricPoint) float64 {
		labelKey := getLabelKey(metric.Labels)
		// If the window is not "full" (i.e., we don't have enough preceding points
		// to calculate a rate over the full 'lookback' period), return NaN.
		if len(window) < lookback {
			return math.NaN()
		}

		firstValue := window[0].GetGaugeValue().GetDoubleValue()
		if math.IsNaN(firstValue) {
			firstValue = lastNonNaNMap[labelKey] // Use the tracked value for this label set
		} else {
			lastNonNaNMap[labelKey] = firstValue // Update the tracked value
		}

		lastValue := window[len(window)-1].GetGaugeValue().GetDoubleValue()
		// If the current point (the last value in the window) is NaN, the rate cannot be defined.
		// Propagate NaN to indicate missing data for the result point.
		if math.IsNaN(lastValue) {
			return math.NaN()
		}

		rate := (lastValue - firstValue) / windowSizeSeconds
		return math.Round(rate*100) / 100
	}

	return applySlidingWindow(mf, lookback, rateCalculator)
}

// trimMetricPointsBefore removes metric points older than startMillis from each metric in the MetricFamily.
func trimMetricPointsBefore(mf *metrics.MetricFamily, startMillis int64) *metrics.MetricFamily {
	for _, metric := range mf.Metrics {
		points := metric.MetricPoints
		// Find first index where point >= startMillis
		cutoff := 0
		for ; cutoff < len(points); cutoff++ {
			point := points[cutoff]
			pointMillis := point.Timestamp.Seconds*1000 + int64(point.Timestamp.Nanos)/1000000
			if pointMillis >= startMillis {
				break
			}
		}
		// Slice the array starting from cutoff index
		metric.MetricPoints = points[cutoff:]
	}
	return mf
}

// applySlidingWindow applies a given processing function over a moving window of metric points.
func applySlidingWindow(mf *metrics.MetricFamily, lookback int, processor func(metric *metrics.Metric, window []*metrics.MetricPoint) float64) *metrics.MetricFamily {
	for _, metric := range mf.Metrics {
		points := metric.MetricPoints
		if len(points) == 0 {
			continue
		}

		processedPoints := make([]*metrics.MetricPoint, 0, len(points))

		for i, currentPoint := range points {
			start := i - lookback + 1
			if start < 0 {
				start = 0
			}
			window := points[start : i+1]
			resultValue := processor(metric, window)

			processedPoints = append(processedPoints, &metrics.MetricPoint{
				Timestamp: currentPoint.Timestamp,
				Value:     toDomainMetricPointValue(resultValue),
			})
		}
		metric.MetricPoints = processedPoints
	}
	return mf
}

func scaleToMillisAndRound(_ *metrics.Metric, window []*metrics.MetricPoint) float64 {
	if len(window) == 0 {
		return math.NaN()
	}

	v := window[len(window)-1].GetGaugeValue().GetDoubleValue()
	// Scale down the value (e.g., from microseconds to milliseconds)
	resultValue := v / 1000.0
	return math.Round(resultValue*100) / 100 // Round to 2 decimal places
}
