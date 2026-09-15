// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

func serviceIs(name string) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
		&expression.StringValue{Value: name},
	}}
}

func attributesWith(key, value string) pcommon.Map {
	attributes := pcommon.NewMap()
	attributes.PutStr(key, value)
	return attributes
}

// TestEnsureFilterStandsAlone covers the mutual exclusion each wire owes a Reader: a query carries
// the filter or the fields it replaces, never both, because the two express the same predicates and
// a Reader given both would answer to one of them without saying which.
func TestEnsureFilterStandsAlone(t *testing.T) {
	tests := []struct {
		name    string
		query   TraceQueryParams
		wantMsg string
	}{
		{
			name:  "no filter, so the legacy fields are the query",
			query: TraceQueryParams{ServiceName: "cart", OperationName: "checkout"},
		},
		{
			name:  "a filter alone",
			query: TraceQueryParams{Filter: serviceIs("cart")},
		},
		{
			name:    "a filter beside a service name",
			query:   TraceQueryParams{Filter: serviceIs("cart"), ServiceName: "cart"},
			wantMsg: "[service_name]",
		},
		{
			name:    "a filter beside an operation name",
			query:   TraceQueryParams{Filter: serviceIs("cart"), OperationName: "checkout"},
			wantMsg: "[operation_name]",
		},
		{
			name:    "a filter beside a duration bound",
			query:   TraceQueryParams{Filter: serviceIs("cart"), DurationMin: time.Second},
			wantMsg: "[duration_min]",
		},
		{
			name:    "a filter beside the other duration bound",
			query:   TraceQueryParams{Filter: serviceIs("cart"), DurationMax: time.Second},
			wantMsg: "[duration_max]",
		},
		{
			name:    "a filter beside the attributes map",
			query:   TraceQueryParams{Filter: serviceIs("cart"), Attributes: attributesWith("k", "v")},
			wantMsg: "[attributes]",
		},
		{
			name:  "a filter beside an empty attributes map, which is no predicate",
			query: TraceQueryParams{Filter: serviceIs("cart"), Attributes: pcommon.NewMap()},
		},
		{
			name: "every field at once, all of them named",
			query: TraceQueryParams{
				Filter: serviceIs("cart"), ServiceName: "cart", OperationName: "checkout",
				DurationMin: time.Second, DurationMax: 2 * time.Second,
				Attributes: attributesWith("k", "v"),
			},
			wantMsg: "[service_name operation_name duration_min duration_max attributes]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.query.EnsureFilterStandsAlone()
			if test.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrFilterInvalid)
			require.ErrorContains(t, err, test.wantMsg)
		})
	}
}

// TestDecodePagination covers the wire-scalar decode both api_v3 and storage/v2 share: a
// present-but-zero page_size is refused rather than read as "no Pagination", and a page_size over
// MaxPageSize is clamped down rather than refused (RFC 0014 §4).
func TestDecodePagination(t *testing.T) {
	tests := []struct {
		name       string
		pageSize   uint32
		pageToken  string
		want       Pagination
		wantErrMsg string
	}{
		{
			name:     "a page size alone",
			pageSize: 50,
			want:     Pagination{PageSize: 50},
		},
		{
			name:      "a page size with a continuation token",
			pageSize:  50,
			pageToken: "cursor",
			want:      Pagination{PageSize: 50, PageToken: "cursor"},
		},
		{
			name:       "zero page size is refused",
			pageSize:   0,
			pageToken:  "cursor",
			wantErrMsg: "page_size is required whenever pagination is present",
		},
		{
			name:     "a page size over the maximum is clamped",
			pageSize: MaxPageSize + 1,
			want:     Pagination{PageSize: MaxPageSize},
		},
		{
			name:     "a page size at the maximum is untouched",
			pageSize: MaxPageSize,
			want:     Pagination{PageSize: MaxPageSize},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodePagination(test.pageSize, test.pageToken)
			if test.wantErrMsg != "" {
				require.ErrorIs(t, err, ErrPaginationInvalid)
				require.ErrorContains(t, err, test.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

// TestEnsurePaginationStandsAlone covers the mutual exclusion Pagination owes a Reader: page_size
// replaces search_depth rather than falling back to it, so a query carries one bound or the
// other, never both, and a Pagination with no page_size does not describe a page.
func TestEnsurePaginationStandsAlone(t *testing.T) {
	tests := []struct {
		name    string
		query   TraceQueryParams
		wantMsg string
	}{
		{
			name:  "no pagination",
			query: TraceQueryParams{SearchDepth: 100},
		},
		{
			name:  "pagination alone",
			query: TraceQueryParams{Pagination: Pagination{PageSize: 50}},
		},
		{
			name:    "pagination beside search_depth",
			query:   TraceQueryParams{SearchDepth: 100, Pagination: Pagination{PageSize: 50}},
			wantMsg: "cannot be combined with search_depth",
		},
		{
			name:    "pagination with no page_size",
			query:   TraceQueryParams{Pagination: Pagination{PageToken: "cursor"}},
			wantMsg: "page_size is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.query.EnsurePaginationStandsAlone()
			if test.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrPaginationInvalid)
			require.ErrorContains(t, err, test.wantMsg)
		})
	}
}

// TestEnsureNoPaginationOnFindTraces covers the one check that cannot live in the shared
// prepareSearchQuery path: FindTraces streams whole traces with no field to carry a
// continuation token, so it refuses Pagination outright rather than silently dropping it.
func TestEnsureNoPaginationOnFindTraces(t *testing.T) {
	t.Run("no pagination", func(t *testing.T) {
		require.NoError(t, TraceQueryParams{}.EnsureNoPaginationOnFindTraces())
	})

	t.Run("pagination present", func(t *testing.T) {
		err := TraceQueryParams{Pagination: Pagination{PageSize: 50}}.EnsureNoPaginationOnFindTraces()
		require.ErrorIs(t, err, ErrPaginationUnsupportedByFindTraces)
	})
}

// TestApplyPaginationCapability covers the capability-based degradation: a Reader that declares
// Paginated gets Pagination as sent, one that does not gets PageSize folded into SearchDepth and
// a PageToken refused outright, since it cannot have minted a token it cannot interpret.
func TestApplyPaginationCapability(t *testing.T) {
	t.Run("no pagination is left alone", func(t *testing.T) {
		query := TraceQueryParams{ServiceName: "cart"}
		applied, err := query.ApplyPaginationCapability(SearchCapabilities{})
		require.NoError(t, err)
		assert.Equal(t, query, applied)
	})

	t.Run("a paginating reader gets pagination as sent", func(t *testing.T) {
		query := TraceQueryParams{Pagination: Pagination{PageSize: 50, PageToken: "cursor"}}
		applied, err := query.ApplyPaginationCapability(SearchCapabilities{Paginated: true})
		require.NoError(t, err)
		assert.Equal(t, query, applied)
	})

	t.Run("a non-paginating reader gets page_size folded into search_depth", func(t *testing.T) {
		query := TraceQueryParams{Pagination: Pagination{PageSize: 50}}
		applied, err := query.ApplyPaginationCapability(SearchCapabilities{})
		require.NoError(t, err)
		assert.Equal(t, TraceQueryParams{SearchDepth: 50}, applied)
	})

	t.Run("a page token against a non-paginating reader is refused", func(t *testing.T) {
		query := TraceQueryParams{Pagination: Pagination{PageSize: 50, PageToken: "cursor"}}
		_, err := query.ApplyPaginationCapability(SearchCapabilities{})
		require.ErrorIs(t, err, ErrPaginationUnsupported)
	})
}

// TestForCapabilities covers the choice between the two filtering models: a Reader that declared
// filter support is given the filter, and one that declared none is given the legacy fields it does
// understand, or a refusal where they cannot carry the filter.
func TestForCapabilities(t *testing.T) {
	filterCapable := SearchCapabilities{Filter: &FilterCapabilities{
		Levels:    []expression.Level{expression.LevelResource},
		Operators: []expression.Operator{expression.OpEq},
	}}

	t.Run("no filter is left alone", func(t *testing.T) {
		query := TraceQueryParams{ServiceName: "cart"}
		prepared, err := query.ForCapabilities(SearchCapabilities{})
		require.NoError(t, err)
		assert.Equal(t, query, prepared)
	})

	t.Run("a reader that evaluates filters is given the filter", func(t *testing.T) {
		query := TraceQueryParams{Filter: serviceIs("cart")}
		prepared, err := query.ForCapabilities(filterCapable)
		require.NoError(t, err)
		assert.Equal(t, query, prepared)
	})

	t.Run("a reader that evaluates none is given the legacy fields", func(t *testing.T) {
		query := TraceQueryParams{Filter: serviceIs("cart")}
		prepared, err := query.ForCapabilities(SearchCapabilities{})
		require.NoError(t, err)
		assert.Equal(t, "cart", prepared.ServiceName)
		assert.Nil(t, prepared.Filter)
	})

	t.Run("a filter the legacy fields cannot carry is refused", func(t *testing.T) {
		disjunction := &expression.Call{Op: expression.OpOr, Args: []expression.Expression{
			serviceIs("cart"), serviceIs("checkout"),
		}}
		_, err := TraceQueryParams{Filter: disjunction}.ForCapabilities(SearchCapabilities{})
		require.ErrorIs(t, err, ErrFilterUnsupported)
	})

	t.Run("a predicate the reader did not declare is refused", func(t *testing.T) {
		spanLevel := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
			&expression.AnyValue{Value: "/cart"},
		}}
		_, err := TraceQueryParams{Filter: spanLevel}.ForCapabilities(filterCapable)
		require.ErrorIs(t, err, ErrFilterUnsupported)
		require.ErrorContains(t, err, `it does not index the "span" level`)
	})
}

// TestEnsureSupported walks the shapes the declaration is read against, since a predicate refused
// here is one that would otherwise reach a Reader that cannot evaluate it.
func TestEnsureSupported(t *testing.T) {
	caps := FilterCapabilities{
		Levels:    []expression.Level{expression.LevelSpan},
		Operators: []expression.Operator{expression.OpAnd, expression.OpEq},
	}

	t.Run("no filter", func(t *testing.T) {
		require.NoError(t, caps.EnsureSupported(nil))
	})

	t.Run("an operator and level both declared", func(t *testing.T) {
		require.NoError(t, caps.EnsureSupported(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
			&expression.AnyValue{Value: "/cart"},
		}}))
	})

	t.Run("an unqualified reference always reaches the reader", func(t *testing.T) {
		require.NoError(t, caps.EnsureSupported(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.route"},
			&expression.AnyValue{Value: "/cart"},
		}}))
	})

	t.Run("an operator not declared", func(t *testing.T) {
		err := caps.EnsureSupported(&expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
			&expression.AnyValue{Value: "/cart/.*"},
		}})
		require.ErrorContains(t, err, `it does not support the operator "regex"`)
	})

	t.Run("a nested predicate is walked too", func(t *testing.T) {
		err := caps.EnsureSupported(&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
			&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
				&expression.AnyValue{Value: "/cart"},
			}},
			&expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
				&expression.AnyValue{Value: "/cart/.*"},
			}},
		}})
		require.ErrorContains(t, err, `it does not support the operator "regex"`)
	})

	t.Run("a collection reference carries a level like any other", func(t *testing.T) {
		err := caps.EnsureSupported(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.NestedRef{Level: expression.LevelEvent},
			&expression.AnyValue{Value: "x"},
		}})
		require.ErrorContains(t, err, `it does not index the "event" level`)
	})
}
