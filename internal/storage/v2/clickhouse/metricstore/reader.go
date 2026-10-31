// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/gogo/protobuf/types"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

var _ metricstore.Reader = (*Reader)(nil)

const (
	defaultLookback    = time.Hour
	defaultStepSeconds = uint64(60)
)

type Reader struct {
	conn driver.Conn
}

func NewReader(conn driver.Conn) *Reader {
	return &Reader{conn: conn}
}

func (r *Reader) GetLatencies(ctx context.Context, params *metricstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	base := params.BaseQueryParameters
	start, end := queryWindow(base)
	step := stepSeconds(base)
	kinds := convertSpanKinds(base.SpanKinds)
	return r.queryMetrics(ctx, metricsQuery{
		baseName:  "service_latencies",
		opName:    "service_operation_latencies",
		baseDesc:  fmt.Sprintf("%.2fth quantile latency, grouped by service", params.Quantile),
		baseQuery: sql.SelectLatencies,
		opQuery:   sql.SelectLatenciesByOperation,
		args:      []any{step, params.Quantile, start, end, base.ServiceNames, kinds},
	}, base)
}

// GetCallRates returns calls/sec, averaged over the caller's RatePer window.
//
// When RatePer is unset or no wider than Step, this is unchanged from
// before: one query bucket, one output point, divided straight into a
// per-second rate by the database.
//
// When RatePer is wider than Step, a single bucketed query can't produce the
// result: RatePer and Step are different things (RatePer is the rate's
// averaging window, Step is just the output resolution - see
// BaseQueryParameters' doc comments), so a single query bucket can't carry
// both. That path instead fetches a raw call count per Step-wide bucket and
// windowCallRates sums 'lookback' consecutive buckets per output point to
// apply the RatePer window, the same lookback-buckets approach the
// Elasticsearch backend uses in calcCallRate
// (internal/storage/metricstore/elasticsearch/processor.go), adapted here
// for ClickHouse's per-bucket counts instead of ES's cumulative counter
// values.
func (r *Reader) GetCallRates(ctx context.Context, params *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	base := params.BaseQueryParameters
	start, end := queryWindow(base)
	step := stepSeconds(base)
	lookback := lookbackBuckets(step, base.RatePer)
	kinds := convertSpanKinds(base.SpanKinds)

	if lookback <= 1 {
		return r.queryMetrics(ctx, metricsQuery{
			baseName:  "service_call_rate",
			opName:    "service_operation_call_rate",
			baseDesc:  "calls/sec, grouped by service",
			baseQuery: sql.SelectCallRates,
			opQuery:   sql.SelectCallRatesByOperation,
			args:      []any{step, step, start, end, base.ServiceNames, kinds},
		}, base)
	}

	name, desc := "service_call_rate", "calls/sec, grouped by service"
	query := sql.SelectCallCounts
	if base.GroupByOperation {
		name, desc = "service_operation_call_rate", "calls/sec, grouped by service & operation"
		query = sql.SelectCallCountsByOperation
	}
	queryStart := start.Add(-time.Duration(lookback-1) * time.Duration(step) * time.Second)

	rows, err := r.conn.Query(ctx, query, step, queryStart, end, base.ServiceNames, kinds)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", name, err)
	}
	counts, err := scanMetricsRows(rows, base.GroupByOperation)
	if err != nil {
		return nil, err
	}
	rates := windowCallRates(counts, step, lookback, queryStart, start, end)
	return groupedToMetricFamily(rates, name, desc, base.GroupByOperation), nil
}

// lookbackBuckets converts a RatePer window into a number of Step-wide
// buckets to sum over. A nil RatePer, or one no wider than Step (no
// meaningful smoothing requested), keeps the pre-existing behavior of a
// single, unsmoothed bucket.
func lookbackBuckets(stepSecs uint64, ratePer *time.Duration) int {
	if ratePer == nil || stepSecs == 0 {
		return 1
	}
	lookback := int(math.Ceil(ratePer.Seconds() / float64(stepSecs)))
	if lookback < 1 {
		return 1
	}
	return lookback
}

// windowCallRates turns raw per-bucket call counts into a rate by summing
// 'lookback' consecutive Step-wide buckets ending at each output point, then
// dividing by the window's length in seconds. Only called when lookback > 1
// (a RatePer wider than Step was actually requested); it walks a dense,
// Step-aligned tick sequence from queryStart to end rather than just the
// ticks ClickHouse actually returned a row for, so a quiet bucket (no spans
// in that Step window) correctly counts as zero calls instead of being
// skipped, which would otherwise shrink the window's real wall-clock span.
func windowCallRates(
	counts map[metricsKey][]*metrics.MetricPoint,
	stepSecs uint64,
	lookback int,
	queryStart, start, end time.Time,
) map[metricsKey][]*metrics.MetricPoint {
	step := int64(stepSecs)
	firstTick := alignDown(queryStart.Unix(), step)
	lastTick := alignDown(end.Add(-time.Nanosecond).Unix(), step)
	windowSeconds := float64(lookback) * float64(stepSecs)

	out := make(map[metricsKey][]*metrics.MetricPoint, len(counts))
	for key, points := range counts {
		byTick := make(map[int64]float64, len(points))
		for _, p := range points {
			byTick[p.GetTimestamp().GetSeconds()] = p.GetGaugeValue().GetDoubleValue()
		}

		var windowed []*metrics.MetricPoint
		history := make([]float64, 0, lookback)
		for tick := firstTick; tick <= lastTick; tick += step {
			history = append(history, byTick[tick])
			if len(history) > lookback {
				history = history[len(history)-lookback:]
			}
			if len(history) < lookback {
				continue
			}
			if time.Unix(tick, 0).UTC().Before(start) {
				continue
			}
			var sum float64
			for _, c := range history {
				sum += c
			}
			windowed = append(windowed, &metrics.MetricPoint{
				Timestamp: &types.Timestamp{Seconds: tick},
				Value: &metrics.MetricPoint_GaugeValue{
					GaugeValue: &metrics.GaugeValue{
						Value: &metrics.GaugeValue_DoubleValue{DoubleValue: sum / windowSeconds},
					},
				},
			})
		}
		if len(windowed) > 0 {
			out[key] = windowed
		}
	}
	return out
}

// alignDown rounds unixSeconds down to the nearest multiple of stepSecs,
// matching ClickHouse's toStartOfInterval(..., toIntervalSecond(stepSecs))
// bucket boundaries (epoch-aligned, not aligned to the query's own start
// time) so the dense tick sequence in windowCallRates lines up with the
// tick values ClickHouse actually returns.
func alignDown(unixSeconds, stepSecs int64) int64 {
	return (unixSeconds / stepSecs) * stepSecs
}

func (r *Reader) GetErrorRates(ctx context.Context, params *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	base := params.BaseQueryParameters
	start, end := queryWindow(base)
	step := stepSeconds(base)
	kinds := convertSpanKinds(base.SpanKinds)
	return r.queryMetrics(ctx, metricsQuery{
		baseName:  "service_error_rate",
		opName:    "service_operation_error_rate",
		baseDesc:  "error rate, grouped by service",
		baseQuery: sql.SelectErrorRates,
		opQuery:   sql.SelectErrorRatesByOperation,
		args:      []any{step, start, end, base.ServiceNames, kinds},
	}, base)
}

type metricsQuery struct {
	baseName  string
	opName    string
	baseDesc  string
	baseQuery string
	opQuery   string
	args      []any
}

// queryMetrics executes a metrics query against ClickHouse and returns the
// result as a MetricFamily.
func (r *Reader) queryMetrics(
	ctx context.Context,
	cfg metricsQuery,
	params metricstore.BaseQueryParameters,
) (*metrics.MetricFamily, error) {
	name, desc := cfg.baseName, cfg.baseDesc
	query := cfg.baseQuery
	if params.GroupByOperation {
		name = cfg.opName
		desc += " & operation"
		query = cfg.opQuery
	}

	rows, err := r.conn.Query(ctx, query, cfg.args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", name, err)
	}
	return rowsToMetricFamily(rows, name, desc, params.GroupByOperation)
}

// metricsKey groups metric points by service (and optionally operation).
type metricsKey struct {
	ServiceName string
	Operation   string
}

// rowsToMetricFamily reads aggregated rows from ClickHouse and converts them to a MetricFamily.
// When groupByOperation is true, rows are expected to have 4 columns: (ts, service_name, name, val).
// Otherwise, rows have 3 columns: (ts, service_name, val).
func rowsToMetricFamily(rows driver.Rows, name, desc string, groupByOperation bool) (*metrics.MetricFamily, error) {
	grouped, err := scanMetricsRows(rows, groupByOperation)
	if err != nil {
		return nil, err
	}
	return groupedToMetricFamily(grouped, name, desc, groupByOperation), nil
}

// scanMetricsRows reads aggregated rows from ClickHouse into points grouped by
// (service, operation). When groupByOperation is true, rows are expected to
// have 4 columns: (ts, service_name, name, val). Otherwise, rows have 3
// columns: (ts, service_name, val).
func scanMetricsRows(rows driver.Rows, groupByOperation bool) (map[metricsKey][]*metrics.MetricPoint, error) {
	defer rows.Close()

	grouped := make(map[metricsKey][]*metrics.MetricPoint)
	for rows.Next() {
		var row metricsRow
		var err error
		if groupByOperation {
			err = rows.Scan(&row.Timestamp, &row.ServiceName, &row.Operation, &row.Value)
		} else {
			err = rows.Scan(&row.Timestamp, &row.ServiceName, &row.Value)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to scan metrics row: %w", err)
		}
		key := metricsKey{ServiceName: row.ServiceName}
		if groupByOperation {
			key.Operation = row.Operation
		}
		grouped[key] = append(grouped[key], row.toMetricPoint())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics rows: %w", err)
	}
	return grouped, nil
}

// groupedToMetricFamily assembles points already grouped by (service, operation) into a MetricFamily.
func groupedToMetricFamily(grouped map[metricsKey][]*metrics.MetricPoint, name, desc string, groupByOperation bool) *metrics.MetricFamily {
	ms := make([]*metrics.Metric, 0, len(grouped))
	for key, points := range grouped {
		labels := []*metrics.Label{
			{Name: "service_name", Value: key.ServiceName},
		}
		if groupByOperation {
			labels = append(labels, &metrics.Label{Name: "operation", Value: key.Operation})
		}
		ms = append(ms, &metrics.Metric{
			Labels:       labels,
			MetricPoints: points,
		})
	}

	return &metrics.MetricFamily{
		Name:    name,
		Type:    metrics.MetricType_GAUGE,
		Help:    desc,
		Metrics: ms,
	}
}

// queryWindow extracts the start and end time from BaseQueryParameters.
func queryWindow(p metricstore.BaseQueryParameters) (start, end time.Time) {
	end = time.Now().UTC()
	if p.EndTime != nil {
		end = p.EndTime.UTC()
	}
	lookback := defaultLookback
	if p.Lookback != nil {
		lookback = *p.Lookback
	}
	start = end.Add(-lookback)
	return start, end
}

// stepSeconds extracts the step duration in seconds from BaseQueryParameters.
func stepSeconds(p metricstore.BaseQueryParameters) uint64 {
	if p.Step != nil && *p.Step > 0 {
		s := *p.Step / time.Second
		if s < 1 {
			return 1
		}
		return uint64(s)
	}
	return defaultStepSeconds
}

func convertSpanKinds(kinds []string) []string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		// SpanKindUnspecified is stored as "" in ClickHouse (via jptrace.SpanKindToString).
		if k == "SPAN_KIND_UNSPECIFIED" {
			out = append(out, "")
			continue
		}
		converted := jptrace.ProtoSpanKindToString(k)
		if converted != "" {
			out = append(out, converted)
		}
	}
	return out
}
