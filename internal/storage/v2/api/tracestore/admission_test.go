// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

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
