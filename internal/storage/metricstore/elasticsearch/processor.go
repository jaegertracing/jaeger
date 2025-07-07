package elasticsearch

import (
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"math"
)

// ProcessLatencies processes raw latency metrics by applying scaling and rounding.
func ProcessLatencies(mf *metrics.MetricFamily) *metrics.MetricFamily {
	const lookback = 1 // only current value
	return applySlidingWindow(mf, lookback, scaleToMillisAndRound)
}

// ProcessCallRates processes raw call rate metrics by calculating rates and trimming to time range.
func ProcessCallRates(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters, timeRange TimeRange) *metrics.MetricFamily {
	processed := calcCallRate(mf, params)
	return trimMetricPointsBefore(processed, timeRange.startTimeMillis)
}

// ProcessErrorRates processes error rate metrics by computing error rates from errors and calls.
func ProcessErrorRates(rawErrors, calls *metrics.MetricFamily, params metricstore.BaseQueryParameters, timeRange TimeRange) *metrics.MetricFamily {
	processedErrors := ProcessCallRates(rawErrors, params, timeRange)
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

	for i, errorMetric := range errorMetrics.Metrics {
		if i >= len(callMetrics.Metrics) {
			break
		}

		callMetric := callMetrics.Metrics[i]
		metricPoints := calculateErrorRatePoints(errorMetric.MetricPoints, callMetric.MetricPoints)

		result.Metrics = append(result.Metrics, &metrics.Metric{
			Labels:       errorMetric.Labels,
			MetricPoints: metricPoints,
		})
	}

	return result
}

// calculateErrorRatePoints computes error rates for corresponding metric points.
func calculateErrorRatePoints(errorPoints, callPoints []*metrics.MetricPoint) []*metrics.MetricPoint {
	metricPoints := make([]*metrics.MetricPoint, 0, len(errorPoints))

	for j, errorPoint := range errorPoints {
		if j >= len(callPoints) {
			break
		}

		callPoint := callPoints[j]
		value := calculateErrorRateValue(errorPoint, callPoint)

		metricPoints = append(metricPoints, &metrics.MetricPoint{
			Timestamp: errorPoint.Timestamp,
			Value:     toDomainMetricPointValue(value),
		})
	}

	return metricPoints
}

// calculateErrorRateValue computes the error rate for a single point.
func calculateErrorRateValue(errorPoint, callPoint *metrics.MetricPoint) float64 {
	errorValue := errorPoint.GetGaugeValue().GetDoubleValue()
	callValue := callPoint.GetGaugeValue().GetDoubleValue()

	if math.IsNaN(errorValue) && !math.IsNaN(callValue) {
		return 0.0
	}

	if math.IsNaN(errorValue) || math.IsNaN(callValue) {
		return math.NaN()
	}
	return errorValue / callValue
}

// calcCallRate defines the rate calculation logic and passes it to applySlidingWindow.
func calcCallRate(mf *metrics.MetricFamily, params metricstore.BaseQueryParameters) *metrics.MetricFamily {
	lookback := int(math.Ceil(float64(params.RatePer.Milliseconds()) / float64(params.Step.Milliseconds())))
	lookback = int(math.Max(float64(lookback), 1))

	windowSizeSeconds := float64(lookback) * params.Step.Seconds()

	rateCalculator := func(window []*metrics.MetricPoint) float64 {
		if len(window) < lookback {
			return math.NaN()
		}

		firstValue := window[0].GetGaugeValue().GetDoubleValue()
		if math.IsNaN(firstValue) {
			firstValue = 0
		}
		lastValue := window[len(window)-1].GetGaugeValue().GetDoubleValue()
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
		cutoff := 0
		for ; cutoff < len(points); cutoff++ {
			point := points[cutoff]
			pointMillis := point.Timestamp.Seconds*1000 + int64(point.Timestamp.Nanos)/1000000
			if pointMillis >= startMillis {
				break
			}
		}
		metric.MetricPoints = points[cutoff:]
	}
	return mf
}

// applySlidingWindow applies a given processing function over a moving window of metric points.
func applySlidingWindow(mf *metrics.MetricFamily, lookback int, processor func(window []*metrics.MetricPoint) float64) *metrics.MetricFamily {
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
			resultValue := processor(window)

			processedPoints = append(processedPoints, &metrics.MetricPoint{
				Timestamp: currentPoint.Timestamp,
				Value:     toDomainMetricPointValue(resultValue),
			})
		}
		metric.MetricPoints = processedPoints
	}
	return mf
}

// scaleToMillisAndRound scales the metric value to milliseconds and rounds it.
func scaleToMillisAndRound(window []*metrics.MetricPoint) float64 {
	if len(window) == 0 {
		return math.NaN()
	}

	v := window[len(window)-1].GetGaugeValue().GetDoubleValue()
	resultValue := v / 1000.0
	return math.Round(resultValue*100) / 100
}
