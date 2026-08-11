// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestParseFindTracesQuery(t *testing.T) {
	tMin := time.Now().Add(-time.Hour).UTC().Truncate(time.Nanosecond)
	tMax := time.Now().UTC().Truncate(time.Nanosecond)

	goodMin := tMin.Format(time.RFC3339Nano)
	goodMax := tMax.Format(time.RFC3339Nano)

	t.Run("all params (canonical)", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramServiceName, "svc")
		q.Set(paramOperationName, "op")
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramSearchDepth, "20")
		q.Set(paramDurationMin, "1s")
		q.Set(paramDurationMax, "2s")
		q.Set(paramQueryRawTraces, "true")

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, "svc", got.ServiceName)
		assert.Equal(t, "op", got.OperationName)
		assert.Equal(t, tMin, got.StartTimeMin)
		assert.Equal(t, tMax, got.StartTimeMax)
		assert.Equal(t, 20, got.SearchDepth)
		assert.Equal(t, time.Second, got.DurationMin)
		assert.Equal(t, 2*time.Second, got.DurationMax)
		assert.True(t, got.RawTraces)
	})

	t.Run("all params (deprecated snake_case)", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramServiceNameDeprecated, "svc")
		q.Set(paramOperationNameDeprecated, "op")
		q.Set(paramTimeMinDeprecated, goodMin)
		q.Set(paramTimeMaxDeprecated, goodMax)
		q.Set(paramSearchDepthDeprecated, "5")
		q.Set(paramDurationMinDeprecated, "500ms")
		q.Set(paramDurationMaxDeprecated, "1s")
		q.Set(paramQueryRawTracesDeprecated, "true")

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, "svc", got.ServiceName)
		assert.Equal(t, "op", got.OperationName)
		assert.Equal(t, tMin, got.StartTimeMin)
		assert.Equal(t, tMax, got.StartTimeMax)
		assert.Equal(t, 5, got.SearchDepth)
		assert.Equal(t, 500*time.Millisecond, got.DurationMin)
		assert.Equal(t, time.Second, got.DurationMax)
		assert.True(t, got.RawTraces)
	})

	t.Run("default search depth", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, defaultSearchDepth, got.SearchDepth)
	})

	t.Run("search depth via num_traces alias", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramNumTraces, "7")

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, 7, got.SearchDepth)
	})

	t.Run("attributes", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramAttributes, `{"http.status_code":"200","error":"true"}`)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		want := pcommon.NewMap()
		want.PutStr("http.status_code", "200")
		want.PutStr("error", "true")
		assert.Equal(t, want.AsRaw(), got.Attributes.AsRaw())
	})

	t.Run("no attributes gives empty map", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, 0, got.Attributes.Len())
	})

	errorCases := []struct {
		name    string
		params  map[string]string
		wantErr string
	}{
		{
			name:    "no time range",
			wantErr: "query.startTimeMin and query.startTimeMax are required",
		},
		{
			name:    "no max time",
			params:  map[string]string{paramTimeMin: goodMin},
			wantErr: "query.startTimeMin and query.startTimeMax are required",
		},
		{
			name:    "no min time",
			params:  map[string]string{paramTimeMax: goodMax},
			wantErr: "query.startTimeMin and query.startTimeMax are required",
		},
		{
			name:    "startTimeMin not before startTimeMax",
			params:  map[string]string{paramTimeMin: goodMax, paramTimeMax: goodMin},
			wantErr: paramTimeMin + " must be before " + paramTimeMax,
		},
		{
			name:    "startTimeMin equals startTimeMax",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMin},
			wantErr: paramTimeMin + " must be before " + paramTimeMax,
		},
		{
			name:    "bad startTimeMin (canonical)",
			params:  map[string]string{paramTimeMin: "NaN", paramTimeMax: goodMax},
			wantErr: "malformed parameter " + paramTimeMin,
		},
		{
			name:    "bad start_time_min (deprecated)",
			params:  map[string]string{paramTimeMinDeprecated: "NaN", paramTimeMaxDeprecated: goodMax},
			wantErr: "malformed parameter " + paramTimeMinDeprecated,
		},
		{
			name:    "bad startTimeMax (canonical)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: "NaN"},
			wantErr: "malformed parameter " + paramTimeMax,
		},
		{
			name:    "bad start_time_max (deprecated)",
			params:  map[string]string{paramTimeMinDeprecated: goodMin, paramTimeMaxDeprecated: "NaN"},
			wantErr: "malformed parameter " + paramTimeMaxDeprecated,
		},
		{
			name:    "bad searchDepth (canonical)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramSearchDepth: "NaN"},
			wantErr: "malformed parameter " + paramSearchDepth,
		},
		{
			name:    "bad search_depth (deprecated)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramSearchDepthDeprecated: "NaN"},
			wantErr: "malformed parameter " + paramSearchDepthDeprecated,
		},
		{
			name:    "bad num_traces (deprecated alias)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramNumTraces: "NaN"},
			wantErr: "malformed parameter " + paramNumTraces,
		},
		{
			name:    "bad durationMin (canonical)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramDurationMin: "NaN"},
			wantErr: "malformed parameter " + paramDurationMin,
		},
		{
			name:    "bad duration_min (deprecated)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramDurationMinDeprecated: "NaN"},
			wantErr: "malformed parameter " + paramDurationMinDeprecated,
		},
		{
			name:    "bad durationMax (canonical)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramDurationMax: "NaN"},
			wantErr: "malformed parameter " + paramDurationMax,
		},
		{
			name:    "bad duration_max (deprecated)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramDurationMaxDeprecated: "NaN"},
			wantErr: "malformed parameter " + paramDurationMaxDeprecated,
		},
		{
			name:    "bad rawTraces (canonical)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramQueryRawTraces: "foobar"},
			wantErr: "malformed parameter " + paramQueryRawTraces,
		},
		{
			name:    "bad raw_traces (deprecated)",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramQueryRawTracesDeprecated: "foobar"},
			wantErr: "malformed parameter " + paramQueryRawTracesDeprecated,
		},
		{
			name:    "bad attributes json",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramAttributes: "not-valid-json"},
			wantErr: "malformed parameter " + paramAttributes,
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			for k, v := range tc.params {
				q.Set(k, v)
			}
			_, err := parseFindTracesQuery(q)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestParseFindTracesQuery_Filter covers the GET binding of the structured filter: a GET
// cannot expand a message by field path without losing the recursive args, so the filter
// travels as one URL-encoded JSON object.
func TestParseFindTracesQuery_Filter(t *testing.T) {
	timeRange := func() url.Values {
		q := url.Values{}
		q.Set(paramTimeMin, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano))
		q.Set(paramTimeMax, time.Now().UTC().Format(time.RFC3339Nano))
		return q
	}

	t.Run("a single predicate", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"eq","args":[{"ref":{"name":"http.status_code"}},{"scalar":{"value":"500"}}]}`)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
			&tracestore.Reference{Name: "http.status_code"},
			&tracestore.Scalar{Value: "500"},
		}}, got.Filter)
	})

	t.Run("a conjunction with a level-qualified attribute and a list", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"and","args":[
			{"call":{"op":"gt","args":[{"ref":{"name":"duration","level":"span"}},{"scalar":{"value":"2s"}}]}},
			{"call":{"op":"in","args":[{"ref":{"name":"http.status_code"}},{"list":{"values":["500","503"],"type":"int"}}]}}]}`)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
			&tracestore.Call{Op: tracestore.OpGt, Args: []tracestore.Expression{
				&tracestore.Reference{Name: tracestore.FieldDuration, Level: tracestore.LevelSpan},
				&tracestore.Scalar{Value: "2s"},
			}},
			&tracestore.Call{Op: tracestore.OpIn, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "http.status_code"},
				&tracestore.List{Values: []string{"500", "503"}, Type: tracestore.ValueTypeInt},
			}},
		}}, got.Filter)
	})

	t.Run("no filter", func(t *testing.T) {
		got, err := parseFindTracesQuery(timeRange())
		require.NoError(t, err)
		assert.Nil(t, got.Filter)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"eq",`)

		_, err := parseFindTracesQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.filter")
	})

	t.Run("a field the schema does not define", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"operator":"eq"}`)

		_, err := parseFindTracesQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.filter")
	})

	t.Run("a filter this build cannot evaluate", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"matches","args":[{"ref":{"name":"a"}},{"scalar":{"value":"b"}}]}`)

		_, err := parseFindTracesQuery(q)
		require.ErrorContains(t, err, `malformed parameter query.filter: unknown filter operator "matches"`)
	})
}

func TestGetQueryParam(t *testing.T) {
	q := url.Values{}
	q.Set("canonical", "c-val")
	q.Set("deprecated", "d-val")

	v, p := getQueryParam(q, "canonical", "deprecated")
	assert.Equal(t, "c-val", v)
	assert.Equal(t, "canonical", p)

	q2 := url.Values{}
	q2.Set("deprecated", "d-val")
	v, p = getQueryParam(q2, "canonical", "deprecated")
	assert.Equal(t, "d-val", v)
	assert.Equal(t, "deprecated", p)

	q3 := url.Values{}
	v, p = getQueryParam(q3, "canonical", "deprecated")
	assert.Empty(t, v)
	assert.Equal(t, "deprecated", p)
}
