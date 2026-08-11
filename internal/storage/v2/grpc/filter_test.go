// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"

	storage "github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TestFilterRoundTrip covers both directions of the remote boundary at once: what the client
// encodes, the server must decode back to the same filter, or a predicate would change
// meaning in transit.
func TestFilterRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		filter *tracestore.Call
	}{
		{
			name: "no filter",
		},
		{
			name: "an unqualified attribute equality",
			filter: &tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "http.status_code"},
				&tracestore.Scalar{Value: "500"},
			}},
		},
		{
			name: "a level-qualified attribute and a typed constant",
			filter: &tracestore.Call{Op: tracestore.OpGt, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "size", Level: tracestore.LevelResource, Attr: true},
				&tracestore.Scalar{Value: "500", Type: tracestore.ValueTypeInt},
			}},
		},
		{
			name: "a nested boolean tree with a list operand",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				&tracestore.Call{Op: tracestore.OpNot, Args: []tracestore.Expression{
					&tracestore.Call{Op: tracestore.OpExists, Args: []tracestore.Expression{
						&tracestore.Reference{Name: "error"},
					}},
				}},
				&tracestore.Call{Op: tracestore.OpIn, Args: []tracestore.Expression{
					&tracestore.Reference{Name: "http.status_code"},
					&tracestore.List{Values: []string{"500", "503"}, Type: tracestore.ValueTypeInt},
				}},
			}},
		},
		{
			name: "a correlated match over the event collection",
			filter: &tracestore.Call{Op: tracestore.OpSome, Args: []tracestore.Expression{
				&tracestore.Reference{Level: tracestore.LevelEvent},
				&tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
					&tracestore.Reference{Name: tracestore.FieldName, Level: tracestore.LevelEvent},
					&tracestore.Scalar{Value: "exception"},
				}},
			}},
		},
		{
			name: "a comparison of two references",
			filter: &tracestore.Call{Op: tracestore.OpNe, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "enduser.id", Level: tracestore.LevelSpan, Attr: true},
				&tracestore.Reference{Name: "enduser.id", Level: tracestore.LevelResource, Attr: true},
			}},
		},
		{
			// The value sets are open, so a build that does not know an operator still relays it.
			name: "an operator this build does not know",
			filter: &tracestore.Call{Op: "json_extract", Args: []tracestore.Expression{
				&tracestore.Reference{Name: "input"},
				&tracestore.Scalar{Value: "guardrails[0].is_passed"},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.filter, fromProtoFilter(toProtoFilter(test.filter)))
		})
	}
}

// TestFromProtoFilter_EmptyTerms covers what a client can put on the wire that the AST has no
// place for: an expression with no term set, and a call argument whose call is absent. Both
// drop out, leaving a filter the reader refuses rather than one it misreads.
func TestFromProtoFilter_EmptyTerms(t *testing.T) {
	tests := []struct {
		name     string
		proto    *storage.Call
		expected *tracestore.Call
	}{
		{
			name: "an argument with no term",
			proto: &storage.Call{Op: "eq", Args: []*storage.Expression{
				{Term: &storage.Expression_Ref{Ref: &storage.Reference{Name: "a"}}},
				{},
			}},
			expected: &tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "a"},
			}},
		},
		{
			name: "a call argument with no call",
			proto: &storage.Call{Op: "and", Args: []*storage.Expression{
				{Term: &storage.Expression_Call{Call: nil}},
			}},
			expected: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, fromProtoFilter(test.proto))
		})
	}
}

// TestToProtoExpression_UnknownTerm pins that an expression outside the four term types — only
// reachable as a nil interface, since the interface is closed — encodes as an empty term
// rather than panicking.
func TestToProtoExpression_UnknownTerm(t *testing.T) {
	assert.Equal(t, &storage.Expression{}, toProtoExpression(nil))
}

// TestQueryParametersCarryTheFilter pins the filter onto the query parameters themselves,
// which is what makes a remote backend that declares filter support actually receive one.
func TestQueryParametersCarryTheFilter(t *testing.T) {
	filter := &tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
		&tracestore.Reference{Name: "http.route", Level: tracestore.LevelSpan, Attr: true},
		&tracestore.Scalar{Value: "/cart"},
	}}

	sent := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap(), Filter: filter})
	assert.Equal(t, toProtoFilter(filter), sent.GetFilter())
	assert.Equal(t, filter, toTraceQueryParams(sent).Filter)

	noFilter := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	assert.Nil(t, noFilter.GetFilter())
	assert.Nil(t, toTraceQueryParams(noFilter).Filter)
}

// TestFilterCapabilitiesRoundTrip covers the declaration crossing the same boundary: a
// backend's levels, operators and boolean support must arrive as declared, since the query
// service refuses filters based on them.
func TestFilterCapabilitiesRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		caps *tracestore.FilterCapabilities
	}{
		{
			name: "no filter support declared",
		},
		{
			name: "declared but at its least capable",
			caps: &tracestore.FilterCapabilities{Levels: []tracestore.Level{}, Operators: []tracestore.Operator{}},
		},
		{
			name: "a flat backend",
			caps: &tracestore.FilterCapabilities{
				Levels:    []tracestore.Level{tracestore.LevelSpan, tracestore.LevelResource, tracestore.LevelEvent},
				Operators: []tracestore.Operator{tracestore.OpEq},
			},
		},
		{
			name: "a fully capable backend",
			caps: &tracestore.FilterCapabilities{
				Levels: []tracestore.Level{
					tracestore.LevelSpan, tracestore.LevelResource, tracestore.LevelInstrumentation,
					tracestore.LevelEvent, tracestore.LevelLink,
				},
				Operators: []tracestore.Operator{
					tracestore.OpEq, tracestore.OpNe, tracestore.OpGt, tracestore.OpLt,
					tracestore.OpGte, tracestore.OpLte, tracestore.OpRegex, tracestore.OpExists,
					tracestore.OpIn, tracestore.OpNotIn, tracestore.OpSome,
				},
				Boolean: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.caps, fromProtoFilterCapabilities(toProtoFilterCapabilities(test.caps)))
		})
	}
}
