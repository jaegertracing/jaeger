// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

var _ metricstore.Reader = (*Reader)(nil)

var errNotImplemented = errors.New("not implemented")

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
	name, desc := "service_latencies", fmt.Sprintf("%.2fth quantile latency, grouped by service", params.Quantile)
	query := sql.SelectLatencies
	if params.GroupByOperation {
		name = "service_operation_latencies"
		desc += " & operation"
		query = sql.SelectLatenciesByOperation
	}

	start, end := queryWindow(params.BaseQueryParameters)
	step := stepSeconds(params.BaseQueryParameters)
	kinds := convertSpanKinds(params.SpanKinds)

	rows, err := r.conn.Query(ctx, query,
		step, params.Quantile, start, end, params.ServiceNames, kinds,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query latencies: %w", err)
	}

	return rowsToMetricFamily(rows, name, desc, params.GroupByOperation)
}

func (*Reader) GetCallRates(_ context.Context, _ *metricstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	return nil, errNotImplemented
}

func (r *Reader) GetErrorRates(ctx context.Context, params *metricstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	name, desc := "service_error_rate", "error rate, grouped by service"
	query := sql.SelectErrorRates
	if params.GroupByOperation {
		name = "service_operation_error_rate"
		desc += " & operation"
		query = sql.SelectErrorRatesByOperation
	}

	start, end := queryWindow(params.BaseQueryParameters)
	step := stepSeconds(params.BaseQueryParameters)
	kinds := convertSpanKinds(params.SpanKinds)

	rows, err := r.conn.Query(ctx, query,
		step, start, end, params.ServiceNames, kinds,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query error rates: %w", err)
	}

	return rowsToMetricFamily(rows, name, desc, params.GroupByOperation)
}

func (*Reader) GetMinStepDuration(_ context.Context, _ *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return 0, errNotImplemented
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
	defer rows.Close()

	// Collect points grouped by (service, operation).
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
	}, nil
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
