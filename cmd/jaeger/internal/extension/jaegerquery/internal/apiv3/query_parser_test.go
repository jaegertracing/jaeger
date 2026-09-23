// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
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

	t.Run("unset search depth is left for the query service to default", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, 0, got.SearchDepth)
	})

	t.Run("an absent or inverted time range is left for the query service to refuse", func(t *testing.T) {
		got, err := parseFindTracesQuery(url.Values{})
		require.NoError(t, err)
		assert.True(t, got.StartTimeMin.IsZero())
		assert.True(t, got.StartTimeMax.IsZero())

		q := url.Values{}
		q.Set(paramTimeMin, goodMax)
		q.Set(paramTimeMax, goodMin)
		got, err = parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.True(t, got.StartTimeMax.Before(got.StartTimeMin))
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

	t.Run("search depth at zero and max", func(t *testing.T) {
		for _, depth := range []int{0, tracestore.MaxSearchDepth} {
			q := url.Values{}
			q.Set(paramTimeMin, goodMin)
			q.Set(paramTimeMax, goodMax)
			q.Set(paramSearchDepth, strconv.Itoa(depth))

			got, err := parseFindTracesQuery(q)
			require.NoError(t, err)
			assert.Equal(t, depth, got.SearchDepth)
		}
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

// TestParseFindTracesQuery_Filter covers the GET binding of the structured filter: the parser
// reads one URL-encoded JSON object, and reports what it cannot read under the parameter's own
// name. The first case doubles as the contract test for the JSON spelling of the AST, since
// this is the only surface where a caller writes it by hand.
func TestParseFindTracesQuery_Filter(t *testing.T) {
	timeRange := func() url.Values {
		q := url.Values{}
		q.Set(paramTimeMin, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano))
		q.Set(paramTimeMax, time.Now().UTC().Format(time.RFC3339Nano))
		return q
	}

	t.Run("a conjunction of a built-in field and an attribute", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"and","args":[
			{"call":{"op":"gt","args":[{"field":{"name":"duration","level":"span"}},{"scalar":{"value":"2s"}}]}},
			{"call":{"op":"in","args":[{"attr":{"key":"http.status_code"}},{"list":{"values":["500","503"],"type":"int"}}]}}]}`)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
			&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
				&expression.AnyValue{Value: "2s"},
			}},
			&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
		}}, got.Filter)
	})

	// The quantifier is the one place the third reference arm is written, and its own level says
	// which collection is bound.
	t.Run("a correlated match over the events", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"some","args":[
			{"nested":{"level":"event"}},
			{"call":{"op":"eq","args":[{"field":{"name":"name","level":"event"}},{"scalar":{"value":"exception","type":"string"}}]}}]}`)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
			&expression.NestedRef{Level: expression.LevelEvent},
			&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Level: expression.LevelEvent, Name: expression.EventFieldName},
				&expression.StringValue{Value: "exception"},
			}},
		}}, got.Filter)
	})

	t.Run("a constant that is not the type it declares", func(t *testing.T) {
		q := timeRange()
		q.Set(paramFilter, `{"op":"eq","args":[
			{"attr":{"key":"size"}},{"scalar":{"value":"large","type":"int"}}]}`)

		_, err := parseFindTracesQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.filter")
		require.ErrorContains(t, err, `"large" is not the "int" it declares`)
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
}

func TestParseFindSpansQuery(t *testing.T) {
	tMin := time.Now().Add(-time.Hour).UTC().Truncate(time.Nanosecond)
	tMax := time.Now().UTC().Truncate(time.Nanosecond)
	goodMin := tMin.Format(time.RFC3339Nano)
	goodMax := tMax.Format(time.RFC3339Nano)

	t.Run("time range only", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Equal(t, tMin, got.StartTimeMin)
		assert.Equal(t, tMax, got.StartTimeMax)
		assert.Nil(t, got.Filter)
	})

	t.Run("deprecated snake_case time params are not honored", func(t *testing.T) {
		// This is a new endpoint with no callers to keep the deprecated aliases for.
		q := url.Values{}
		q.Set(paramTimeMinDeprecated, goodMin)
		q.Set(paramTimeMaxDeprecated, goodMax)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.True(t, got.StartTimeMin.IsZero())
		assert.True(t, got.StartTimeMax.IsZero())
	})

	t.Run("a filter", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramFilter, `{"op":"eq","args":[
			{"attr":{"key":"http.route","level":"span"}},{"scalar":{"value":"/cart"}}]}`)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
			&expression.AnyValue{Value: "/cart"},
		}}, got.Filter)
	})

	t.Run("malformed filter JSON", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramFilter, `{"op":"eq",`)

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.filter")
	})

	t.Run("an absent or inverted time range is left for the query service to refuse", func(t *testing.T) {
		got, err := parseFindSpansQuery(url.Values{})
		require.NoError(t, err)
		assert.True(t, got.StartTimeMin.IsZero())
		assert.True(t, got.StartTimeMax.IsZero())

		q := url.Values{}
		q.Set(paramTimeMin, goodMax)
		q.Set(paramTimeMax, goodMin)
		got, err = parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.True(t, got.StartTimeMax.Before(got.StartTimeMin))
	})

	t.Run("malformed min time", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, "not-a-time")
		q.Set(paramTimeMax, goodMax)

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.startTimeMin")
	})

	t.Run("malformed max time", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, "not-a-time")

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.startTimeMax")
	})

	t.Run("undecodable filter", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramFilter, `{"op":"eq","args":[{"attr":{"key":"a"}},{}]}`)

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.filter")
		require.ErrorContains(t, err, "filter argument is empty")
	})

	t.Run("pagination is decoded, not rejected here", func(t *testing.T) {
		// Whether pagination is acceptable at all is the query service's decision
		// (prepareSpanSearchQuery); the parser's job is only to decode the scalars.
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramPageSize, "10")
		q.Set(paramPageToken, "opaque-cursor")

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Equal(t, tracestore.Pagination{PageSize: 10, PageToken: "opaque-cursor"}, got.Pagination)
	})

	t.Run("page token without page size is rejected", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramPageToken, "opaque-cursor")

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "page_size is required")
	})

	t.Run("malformed page size", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramPageSize, "not-a-number")

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.pagination.pageSize")
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
