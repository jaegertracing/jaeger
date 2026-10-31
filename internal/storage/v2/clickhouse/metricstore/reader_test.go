// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

func ptr[T any](v T) *T { return &v }

var testQueryParams = metricstore.BaseQueryParameters{
	ServiceNames: []string{"frontend"},
	EndTime:      ptr(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)),
	Lookback:     ptr(time.Hour),
	Step:         ptr(time.Minute),
	SpanKinds:    []string{"SPAN_KIND_SERVER"},
}

func scanMetricsRowFn(dest any, src metricsRow) error {
	d := dest.([]any)
	*d[0].(*time.Time) = src.Timestamp
	*d[1].(*string) = src.ServiceName
	*d[2].(*float64) = src.Value
	return nil
}

func scanMetricsRowWithOpFn(dest any, src metricsRow) error {
	d := dest.([]any)
	*d[0].(*time.Time) = src.Timestamp
	*d[1].(*string) = src.ServiceName
	*d[2].(*string) = src.Operation
	*d[3].(*float64) = src.Value
	return nil
}

type testCase struct {
	name      string // e.g. "Latencies"
	baseQuery string // SQL query for base case
	opQuery   string // SQL query for GroupByOperation case
	baseName  string // expected MetricFamily.Name for base case
	opName    string // expected MetricFamily.Name for GroupByOperation case
	queryFn   func(*testing.T, *Reader, metricstore.BaseQueryParameters) (*metrics.MetricFamily, error)
}

var testCases = []testCase{
	{
		name:      "Latencies",
		baseQuery: sql.SelectLatencies,
		opQuery:   sql.SelectLatenciesByOperation,
		baseName:  "service_latencies",
		opName:    "service_operation_latencies",
		queryFn: func(t *testing.T, r *Reader, base metricstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
			return r.GetLatencies(t.Context(), &metricstore.LatenciesQueryParameters{
				BaseQueryParameters: base,
				Quantile:            0.95,
			})
		},
	},
	{
		name:      "CallRates",
		baseQuery: sql.SelectCallRates,
		opQuery:   sql.SelectCallRatesByOperation,
		baseName:  "service_call_rate",
		opName:    "service_operation_call_rate",
		queryFn: func(t *testing.T, r *Reader, base metricstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
			return r.GetCallRates(t.Context(), &metricstore.CallRateQueryParameters{
				BaseQueryParameters: base,
			})
		},
	},
	{
		name:      "ErrorRates",
		baseQuery: sql.SelectErrorRates,
		opQuery:   sql.SelectErrorRatesByOperation,
		baseName:  "service_error_rate",
		opName:    "service_operation_error_rate",
		queryFn: func(t *testing.T, r *Reader, base metricstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
			return r.GetErrorRates(t.Context(), &metricstore.ErrorRateQueryParameters{
				BaseQueryParameters: base,
			})
		},
	},
}

func TestMetricStore_SingleService(t *testing.T) {
	ts := time.Date(2025, 1, 1, 11, 30, 0, 0, time.UTC)
	for _, mt := range testCases {
		t.Run(mt.name, func(t *testing.T) {
			driver := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					mt.baseQuery: {
						Rows: &clickhousetest.Rows[metricsRow]{
							Data:   []metricsRow{{Timestamp: ts, ServiceName: "frontend", Value: 1.5}},
							ScanFn: scanMetricsRowFn,
						},
					},
				},
			}
			result, err := mt.queryFn(t, NewReader(driver), testQueryParams)
			require.NoError(t, err)
			require.Equal(t, mt.baseName, result.Name)
			require.Len(t, result.Metrics, 1)
			require.Len(t, result.Metrics[0].Labels, 1)
			assert.Equal(t, "service_name", result.Metrics[0].Labels[0].Name)
			assert.Equal(t, "frontend", result.Metrics[0].Labels[0].Value)
			require.Len(t, result.Metrics[0].MetricPoints, 1)
			assert.InDelta(t, 1.5, result.Metrics[0].MetricPoints[0].GetGaugeValue().GetDoubleValue(), 0.001)
		})
	}
}

func TestMetricStore_MultipleServicesAndBuckets(t *testing.T) {
	ts1 := time.Date(2025, 1, 1, 11, 30, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 1, 11, 31, 0, 0, time.UTC)
	for _, mt := range testCases {
		t.Run(mt.name, func(t *testing.T) {
			driver := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					mt.baseQuery: {
						Rows: &clickhousetest.Rows[metricsRow]{
							Data: []metricsRow{
								{Timestamp: ts1, ServiceName: "frontend", Value: 10.0},
								{Timestamp: ts2, ServiceName: "frontend", Value: 20.0},
								{Timestamp: ts1, ServiceName: "backend", Value: 5.0},
							},
							ScanFn: scanMetricsRowFn,
						},
					},
				},
			}
			params := testQueryParams
			params.ServiceNames = []string{"frontend", "backend"}
			result, err := mt.queryFn(t, NewReader(driver), params)
			require.NoError(t, err)
			require.Equal(t, mt.baseName, result.Name)
			require.Len(t, result.Metrics, 2)

			byService := make(map[string]*metrics.Metric, len(result.Metrics))
			for _, m := range result.Metrics {
				require.Len(t, m.Labels, 1)
				assert.Equal(t, "service_name", m.Labels[0].Name)
				byService[m.Labels[0].Value] = m
			}

			fe := byService["frontend"]
			require.Len(t, fe.MetricPoints, 2)
			assert.InDelta(t, 10.0, fe.MetricPoints[0].GetGaugeValue().GetDoubleValue(), 0.001)
			assert.InDelta(t, 20.0, fe.MetricPoints[1].GetGaugeValue().GetDoubleValue(), 0.001)

			be := byService["backend"]
			require.Len(t, be.MetricPoints, 1)
			assert.InDelta(t, 5.0, be.MetricPoints[0].GetGaugeValue().GetDoubleValue(), 0.001)
		})
	}
}

func TestMetricStore_GroupByOperation(t *testing.T) {
	ts1 := time.Date(2025, 1, 1, 11, 30, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 1, 11, 31, 0, 0, time.UTC)
	for _, mt := range testCases {
		t.Run(mt.name, func(t *testing.T) {
			driver := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					mt.opQuery: {
						Rows: &clickhousetest.Rows[metricsRow]{
							Data: []metricsRow{
								{Timestamp: ts1, ServiceName: "frontend", Operation: "GET /api", Value: 100.0},
								{Timestamp: ts1, ServiceName: "frontend", Operation: "POST /api", Value: 250.0},
								{Timestamp: ts2, ServiceName: "frontend", Operation: "GET /api", Value: 120.0},
							},
							ScanFn: scanMetricsRowWithOpFn,
						},
					},
				},
			}
			params := testQueryParams
			params.GroupByOperation = true
			result, err := mt.queryFn(t, NewReader(driver), params)
			require.NoError(t, err)
			require.Equal(t, mt.opName, result.Name)
			require.Len(t, result.Metrics, 2)

			byOp := make(map[string]*metrics.Metric, len(result.Metrics))
			for _, m := range result.Metrics {
				require.Len(t, m.Labels, 2)
				assert.Equal(t, "service_name", m.Labels[0].Name)
				assert.Equal(t, "frontend", m.Labels[0].Value)
				assert.Equal(t, "operation", m.Labels[1].Name)
				byOp[m.Labels[1].Value] = m
			}

			getAPI := byOp["GET /api"]
			require.Len(t, getAPI.MetricPoints, 2)
			assert.InDelta(t, 100.0, getAPI.MetricPoints[0].GetGaugeValue().GetDoubleValue(), 0.001)
			assert.InDelta(t, 120.0, getAPI.MetricPoints[1].GetGaugeValue().GetDoubleValue(), 0.001)

			postAPI := byOp["POST /api"]
			require.Len(t, postAPI.MetricPoints, 1)
			assert.InDelta(t, 250.0, postAPI.MetricPoints[0].GetGaugeValue().GetDoubleValue(), 0.001)
		})
	}
}

func TestMetricStore_Errors(t *testing.T) {
	errorTests := []struct {
		name     string
		response *clickhousetest.QueryResponse
		err      string
	}{
		{
			name:     "query error",
			response: &clickhousetest.QueryResponse{Err: assert.AnError},
			err:      "failed to query",
		},
		{
			name: "scan error",
			response: &clickhousetest.QueryResponse{
				Rows: &clickhousetest.Rows[metricsRow]{
					Data:    []metricsRow{{ServiceName: "frontend"}},
					ScanErr: assert.AnError,
				},
			},
			err: "failed to scan metrics row",
		},
		{
			name: "rows error",
			response: &clickhousetest.QueryResponse{
				Rows: &clickhousetest.Rows[metricsRow]{
					Data:    []metricsRow{},
					RowsErr: assert.AnError,
				},
			},
			err: "error iterating metrics rows",
		},
	}
	for _, mt := range testCases {
		t.Run(mt.name, func(t *testing.T) {
			for _, tt := range errorTests {
				t.Run(tt.name, func(t *testing.T) {
					t.Cleanup(tt.response.Reset)
					driver := &clickhousetest.Driver{
						QueryResponses: map[string]*clickhousetest.QueryResponse{
							mt.baseQuery: tt.response,
						},
					}
					_, err := mt.queryFn(t, NewReader(driver), testQueryParams)
					require.ErrorContains(t, err, tt.err)
				})
			}
		})
	}
}

func TestStepSeconds(t *testing.T) {
	tests := []struct {
		name string
		step *time.Duration
		want uint64
	}{
		{
			name: "nil step returns default",
			step: nil,
			want: defaultStepSeconds,
		},
		{
			name: "normal step",
			step: ptr(30 * time.Second),
			want: 30,
		},
		{
			name: "sub-second step clamped to 1",
			step: ptr(500 * time.Millisecond),
			want: 1,
		},
		{
			name: "zero step returns default",
			step: ptr(time.Duration(0)),
			want: defaultStepSeconds,
		},
		{
			name: "negative step returns default",
			step: ptr(-5 * time.Second),
			want: defaultStepSeconds,
		},
		{
			name: "one second step",
			step: ptr(time.Second),
			want: 1,
		},
		{
			name: "fractional seconds truncated",
			step: ptr(2500 * time.Millisecond),
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := metricstore.BaseQueryParameters{Step: tt.step}
			assert.Equal(t, tt.want, stepSeconds(p))
		})
	}
}

func TestLookbackBuckets(t *testing.T) {
	tests := []struct {
		name    string
		step    uint64
		ratePer *time.Duration
		want    int
	}{
		{name: "nil RatePer", step: 60, ratePer: nil, want: 1},
		{name: "RatePer narrower than step", step: 60, ratePer: ptr(30 * time.Second), want: 1},
		{name: "RatePer equal to step", step: 60, ratePer: ptr(time.Minute), want: 1},
		{name: "RatePer exact multiple of step", step: 60, ratePer: ptr(3 * time.Minute), want: 3},
		{name: "RatePer rounds up to next step", step: 60, ratePer: ptr(90 * time.Second), want: 2},
		{name: "zero step", step: 0, ratePer: ptr(time.Minute), want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, lookbackBuckets(tt.step, tt.ratePer))
		})
	}
}

func TestAlignDown(t *testing.T) {
	tests := []struct {
		name     string
		unixSecs int64
		step     int64
		want     int64
	}{
		{name: "already aligned", unixSecs: 1735732800, step: 60, want: 1735732800},
		{name: "rounds down to previous bucket", unixSecs: 1735732859, step: 60, want: 1735732800},
		{name: "one second past a bucket", unixSecs: 1735732801, step: 60, want: 1735732800},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, alignDown(tt.unixSecs, tt.step))
		})
	}
}

// TestGetCallRates_NoRatePer_UnchangedBehavior pins down that a nil (or
// narrower-than-step) RatePer takes the exact pre-existing codepath: the
// divided-in-SQL SelectCallRates query, not the new raw-count query.
func TestGetCallRates_NoRatePer_UnchangedBehavior(t *testing.T) {
	ts := time.Date(2025, 1, 1, 11, 30, 0, 0, time.UTC)
	drv := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectCallRates: {
				Rows: &clickhousetest.Rows[metricsRow]{
					Data:   []metricsRow{{Timestamp: ts, ServiceName: "frontend", Value: 1.5}},
					ScanFn: scanMetricsRowFn,
				},
			},
		},
	}
	result, err := NewReader(drv).GetCallRates(t.Context(), &metricstore.CallRateQueryParameters{
		BaseQueryParameters: testQueryParams,
	})
	require.NoError(t, err)
	require.Len(t, drv.RecordedQueries, 1)
	assert.Contains(t, drv.RecordedQueries[0], "count(*) / ?")
	require.Len(t, result.Metrics, 1)
	require.Len(t, result.Metrics[0].MetricPoints, 1)
	assert.InDelta(t, 1.5, result.Metrics[0].MetricPoints[0].GetGaugeValue().GetDoubleValue(), 0.001)
}

// TestGetCallRates_RatePerSmoothing exercises the new windowed path end to
// end: RatePer=3m wider than Step=1m means lookback=3, so the reader should
// query 2 extra 1-minute buckets of history before the requested start, then
// return a rate smoothed over 3-minute (180s) windows.
//
// frontend's raw per-bucket counts, 11:53 through 11:59 (one per minute):
// 3, _, 6, 3, 9, 3, 6 - the 11:54 bucket has no row at all (a quiet minute,
// not a zero explicitly returned by ClickHouse), checking that a gap is
// treated as 0 calls rather than skipped. Hand-computed sums over the
// trailing 3-bucket window, divided by 180s:
//
//	11:55: (3+0+6)/180  = 0.05
//	11:56: (0+6+3)/180  = 0.05
//	11:57: (6+3+9)/180  = 0.1
//	11:58: (3+9+3)/180  = 0.08333...
//	11:59: (9+3+6)/180  = 0.1
//
// 11:53 and 11:54 aren't emitted (not enough history yet), and nothing
// before 11:53 is emitted either (outside the requested start of 11:55).
func TestGetCallRates_RatePerSmoothing(t *testing.T) {
	bucket := func(hh, mm int) time.Time {
		return time.Date(2025, 1, 1, hh, mm, 0, 0, time.UTC)
	}
	drv := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectCallCounts: {
				Rows: &clickhousetest.Rows[metricsRow]{
					Data: []metricsRow{
						{Timestamp: bucket(11, 53), ServiceName: "frontend", Value: 3},
						// 11:54 deliberately omitted - simulates a quiet bucket.
						{Timestamp: bucket(11, 55), ServiceName: "frontend", Value: 6},
						{Timestamp: bucket(11, 56), ServiceName: "frontend", Value: 3},
						{Timestamp: bucket(11, 57), ServiceName: "frontend", Value: 9},
						{Timestamp: bucket(11, 58), ServiceName: "frontend", Value: 3},
						{Timestamp: bucket(11, 59), ServiceName: "frontend", Value: 6},
					},
					ScanFn: scanMetricsRowFn,
				},
			},
		},
	}

	params := metricstore.BaseQueryParameters{
		ServiceNames: []string{"frontend"},
		EndTime:      ptr(bucket(12, 0)),
		Lookback:     ptr(5 * time.Minute),
		Step:         ptr(time.Minute),
		RatePer:      ptr(3 * time.Minute),
		SpanKinds:    []string{"SPAN_KIND_SERVER"},
	}
	result, err := NewReader(drv).GetCallRates(t.Context(), &metricstore.CallRateQueryParameters{
		BaseQueryParameters: params,
	})
	require.NoError(t, err)

	require.Len(t, drv.RecordedQueries, 1)
	assert.Contains(t, drv.RecordedQueries[0], sql.SelectCallCounts)
	recordedArgs := drv.RecordedArgs[0]
	require.Len(t, recordedArgs, 5)
	assert.Equal(t, uint64(60), recordedArgs[0])
	assert.Equal(t, bucket(11, 53), recordedArgs[1], "query should start 2 buckets before the requested start to fill the lookback window")
	assert.Equal(t, bucket(12, 0), recordedArgs[2])

	require.Len(t, result.Metrics, 1)
	points := result.Metrics[0].MetricPoints
	require.Len(t, points, 5)

	wantTimestamps := []time.Time{bucket(11, 55), bucket(11, 56), bucket(11, 57), bucket(11, 58), bucket(11, 59)}
	wantValues := []float64{0.05, 0.05, 0.1, 15.0 / 180.0, 0.1}
	for i, p := range points {
		gotTS := time.Unix(p.GetTimestamp().GetSeconds(), 0).UTC()
		assert.Equal(t, wantTimestamps[i], gotTS, "point %d timestamp", i)
		assert.InDelta(t, wantValues[i], p.GetGaugeValue().GetDoubleValue(), 1e-9, "point %d value", i)
	}
}

func TestConvertSpanKinds(t *testing.T) {
	tests := []struct {
		name  string
		kinds []string
		want  []string
	}{
		{
			name:  "server",
			kinds: []string{"SPAN_KIND_SERVER"},
			want:  []string{"server"},
		},
		{
			name:  "multiple kinds",
			kinds: []string{"SPAN_KIND_SERVER", "SPAN_KIND_CLIENT"},
			want:  []string{"server", "client"},
		},
		{
			name:  "unspecified maps to empty string",
			kinds: []string{"SPAN_KIND_UNSPECIFIED"},
			want:  []string{""},
		},
		{
			name:  "empty input",
			kinds: []string{},
			want:  []string{},
		},
		{
			name:  "unknown kind is skipped",
			kinds: []string{"SPAN_KIND_SERVER", "INVALID_KIND"},
			want:  []string{"server"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, convertSpanKinds(tt.kinds))
		})
	}
}
