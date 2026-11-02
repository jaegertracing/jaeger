// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var p builder.Predicate

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
		assert.EqualValues(t, 20, got.SearchDepth)
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
		assert.EqualValues(t, 5, got.SearchDepth)
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
		assert.EqualValues(t, 0, got.SearchDepth)
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
		assert.EqualValues(t, 7, got.SearchDepth)
	})

	t.Run("search depth at zero and max", func(t *testing.T) {
		for _, depth := range []uint32{0, tracestore.MaxSearchDepth} {
			q := url.Values{}
			q.Set(paramTimeMin, goodMin)
			q.Set(paramTimeMax, goodMax)
			q.Set(paramSearchDepth, strconv.FormatUint(uint64(depth), 10))

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

	t.Run("pagination is decoded as sent, presence included", func(t *testing.T) {
		// Whether a trace search may be paginated, and what an incomplete Pagination means, is
		// the query service's decision (prepareSearchQuery); the parser only preserves presence.
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)

		got, err := parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Nil(t, got.Pagination, "no pagination parameters means not a paginated request")

		q.Set(paramPageSize, "10")
		q.Set(paramPageToken, "opaque-cursor")
		got, err = parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &querysvc.Pagination{PageSize: 10, PageToken: "opaque-cursor"}, got.Pagination)

		q.Del(paramPageSize)
		got, err = parseFindTracesQuery(q)
		require.NoError(t, err)
		assert.Equal(t, &querysvc.Pagination{PageToken: "opaque-cursor"}, got.Pagination,
			"a token alone is still a paginated request; the missing page size is the query service's refusal")
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
			name:    "searchDepth negative",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramSearchDepth: "-1"},
			wantErr: "malformed parameter " + paramSearchDepth,
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
		{
			name:    "bad page size",
			params:  map[string]string{paramTimeMin: goodMin, paramTimeMax: goodMax, paramPageSize: "-1"},
			wantErr: "malformed parameter " + paramPageSize,
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
		assert.Equal(t, p.And(
			p.Span().Duration.Gt("2s"),
			p.Attr("http.status_code").In(p.List(expression.ValueTypeInt, "500", "503")),
		), got.Filter)
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
		assert.Equal(t, p.Some(p.Event(), p.Event().Name.Eq(p.String("exception"))), got.Filter)
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

	t.Run("time range via the deprecated snake_case aliases", func(t *testing.T) {
		// The time range is read by the same helper as a trace search's, aliases included.
		q := url.Values{}
		q.Set(paramTimeMinDeprecated, goodMin)
		q.Set(paramTimeMaxDeprecated, goodMax)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Equal(t, tMin, got.StartTimeMin)
		assert.Equal(t, tMax, got.StartTimeMax)
	})

	t.Run("a filter", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramFilter, `{"op":"eq","args":[
			{"attr":{"key":"http.route","level":"span"}},{"scalar":{"value":"/cart"}}]}`)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Equal(t, p.Span().Attr("http.route").Eq("/cart"), got.Filter)
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
		assert.Equal(t, querysvc.Pagination{PageSize: 10, PageToken: "opaque-cursor"}, got.Pagination)
	})

	t.Run("pagination parameters are optional", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Zero(t, got.Pagination, "the bound is the query service's to fill in")
	})

	t.Run("negative page size", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramTimeMin, goodMin)
		q.Set(paramTimeMax, goodMax)
		q.Set(paramPageSize, "-1")

		_, err := parseFindSpansQuery(q)
		require.ErrorContains(t, err, "malformed parameter query.pagination.pageSize")
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

	v, paramName := getQueryParam(q, "canonical", "deprecated")
	assert.Equal(t, "c-val", v)
	assert.Equal(t, "canonical", paramName)

	q2 := url.Values{}
	q2.Set("deprecated", "d-val")
	v, paramName = getQueryParam(q2, "canonical", "deprecated")
	assert.Equal(t, "d-val", v)
	assert.Equal(t, "deprecated", paramName)

	q3 := url.Values{}
	v, paramName = getQueryParam(q3, "canonical", "deprecated")
	assert.Empty(t, v)
	assert.Equal(t, "deprecated", paramName)
}

const (
	// deepestCallJSON is the call whose JSON nests deepest below its Call object: a list
	// constant's values sit two levels below its argument Expression.
	deepestCallJSON = `{"op":"in","args":[{"attr":{"key":"a"}},{"list":{"values":["x","y"]}}]}`
	// emptyArgCallJSON is a call whose one argument is an empty Expression, one level below its
	// args array.
	emptyArgCallJSON = `{"op":"exists","args":[{}]}`
)

// nestedFilterJSON writes a filter nesting calls deep as protobuf JSON: `not` calls around
// innermost, which is the JSON of the deepest call.
func nestedFilterJSON(calls int, innermost string) string {
	return strings.Repeat(`{"op":"not","args":[{"call":`, calls-1) + innermost + strings.Repeat(`}]}`, calls-1)
}

// sortTermJSON writes query.orderBy holding one sort term over the call in callJSON.
func sortTermJSON(callJSON string) string {
	return `[{"expression":{"call":` + callJSON + `}}]`
}

// jsonDepth is how deeply value nests objects and arrays, as encoding/json reads it, which makes it
// an oracle independent of nestsDeeperThan.
func jsonDepth(t *testing.T, value string) int {
	dec := json.NewDecoder(strings.NewReader(value))
	depth, deepest := 0, 0
	for {
		token, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return deepest
		}
		require.NoError(t, err)
		switch token {
		case json.Delim('{'), json.Delim('['):
			depth++
			deepest = max(deepest, depth)
		case json.Delim('}'), json.Delim(']'):
			depth--
		default:
			// A key or a scalar nests nothing.
		}
	}
}

func TestNestsDeeperThan(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		limit int
		want  bool
	}{
		{"at the limit", `{"a":[1]}`, 2, false},
		{"past the limit", `{"a":[[1]]}`, 2, true},
		{"siblings do not add up", `[[],{"a":1},[[]]]`, 3, false},
		{"brackets inside a string", `["[[{{"]`, 1, false},
		{"an escaped quote does not end a string", `["\"[[{{"]`, 1, false},
		{"an escaped backslash does not escape the quote after it", `["\\",[[1]]]`, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, jsonDepth(t, tc.value) > tc.limit, "the case itself")
			assert.Equal(t, tc.want, nestsDeeperThan(tc.value, tc.limit))
		})
	}
}

// TestParseFilterParam_NestingDepth pins the bound checked before decoding to the AST's own: the
// deepest filter within expression.MaxNestingDepth reaches maxFilterJSONDepth and still decodes,
// and JSON one level deeper is refused without being decoded.
func TestParseFilterParam_NestingDepth(t *testing.T) {
	t.Run("the deepest filter the limit accepts", func(t *testing.T) {
		deepest := nestedFilterJSON(expression.MaxNestingDepth, deepestCallJSON)
		require.Equal(t, maxFilterJSONDepth, jsonDepth(t, deepest))
		q := url.Values{}
		q.Set(paramFilter, deepest)

		got, err := parseFilterParam(q)
		require.NoError(t, err)
		want := p.Attr("a").In("x", "y")
		for range expression.MaxNestingDepth - 1 {
			want = p.Not(want)
		}
		assert.Equal(t, want, got)
	})

	t.Run("one level deeper", func(t *testing.T) {
		tooDeep := nestedFilterJSON(expression.MaxNestingDepth+1, emptyArgCallJSON)
		require.Equal(t, maxFilterJSONDepth+1, jsonDepth(t, tooDeep))
		q := url.Values{}
		q.Set(paramFilter, tooDeep)

		_, err := parseFilterParam(q)
		require.ErrorIs(t, err, errNestedTooDeeply)
		require.ErrorIs(t, err, expression.ErrTooDeeplyNested)
		require.ErrorContains(t, err, "malformed parameter query.filter")
	})
}

// TestParseFindSpansQuery_OrderByNestingDepth does the same for query.orderBy, whose sort terms
// hold an Expression decoded under the same limit.
func TestParseFindSpansQuery_OrderByNestingDepth(t *testing.T) {
	t.Run("a sort term over a built-in field", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramOrderBy, `[{"expression":{"field":{"level":"span","name":"duration"}},"direction":"desc"}]`)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		assert.Equal(t, []tracestore.SpanSortOrder{{
			Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"},
			Direction:  tracestore.SortDescending,
		}}, got.OrderBy)
	})

	// Only a built-in field sorts, but refusing anything else is the query service's decision
	// (tracestore.NormalizeSpanOrder); the parser decodes whatever the depth limit accepts.
	t.Run("the deepest expression the limit accepts", func(t *testing.T) {
		deepest := sortTermJSON(nestedFilterJSON(expression.MaxNestingDepth, deepestCallJSON))
		require.Equal(t, maxOrderByJSONDepth, jsonDepth(t, deepest))
		q := url.Values{}
		q.Set(paramOrderBy, deepest)

		got, err := parseFindSpansQuery(q)
		require.NoError(t, err)
		require.Len(t, got.OrderBy, 1)
		assert.IsType(t, &expression.Call{}, got.OrderBy[0].Expression)
	})

	t.Run("one level deeper", func(t *testing.T) {
		tooDeep := sortTermJSON(nestedFilterJSON(expression.MaxNestingDepth+1, emptyArgCallJSON))
		require.Equal(t, maxOrderByJSONDepth+1, jsonDepth(t, tooDeep))
		q := url.Values{}
		q.Set(paramOrderBy, tooDeep)

		_, err := parseFindSpansQuery(q)
		require.ErrorIs(t, err, errNestedTooDeeply)
		require.ErrorContains(t, err, "malformed parameter query.orderBy")
	})
}

// TestParseFindSpansQuery_RefusesDeepJSONBeforeDecoding is why the depth is checked first. The
// protobuf decoder copies the text below every level it descends, so a long parameter nested
// thousands deep costs gigabytes of allocation before the AST's own bound refuses the tree.
// Checked first, the same parameter costs one read of its first few hundred bytes.
func TestParseFindSpansQuery_RefusesDeepJSONBeforeDecoding(t *testing.T) {
	// About half a megabyte, which a GET query string carries: 2000 calls, about 6000 JSON levels
	// and so inside encoding/json's own limit of 10000, around one long constant.
	deep := nestedFilterJSON(2000, `{"op":"in","args":[{"attr":{"key":"a"}},{"list":{"values":["`+
		strings.Repeat("x", 400_000)+`"]}}]}`)
	for _, param := range []struct{ name, value string }{
		{paramFilter, deep},
		{paramOrderBy, sortTermJSON(deep)},
	} {
		t.Run(param.name, func(t *testing.T) {
			q := url.Values{}
			q.Set(param.name, param.value)

			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := parseFindSpansQuery(q)
			runtime.ReadMemStats(&after)

			// The check allocates nothing; what is allocated is the error, and json.Valid's copy of
			// query.orderBy. 16 MB leaves room for that and the runtime, and is still far below
			// what decoding the parameter allocates.
			assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(16<<20), "bytes allocated")
			require.ErrorIs(t, err, errNestedTooDeeply)
		})
	}
}
