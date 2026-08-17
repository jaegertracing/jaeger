// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// mustToFilter decodes a filter that is expected to be well formed.
func mustToFilter(t *testing.T, filter *Call) *expression.Call {
	t.Helper()
	got, err := ToFilter(filter)
	require.NoError(t, err)
	return got
}

// TestFilterRoundTrip covers both directions of the remote boundary at once: what the client
// encodes, the server must decode back to the same filter, or a predicate would change
// meaning in transit.
func TestFilterRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		filter *expression.Call
	}{
		{
			name: "no filter",
		},
		{
			name: "an unqualified attribute equality",
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.Reference{Name: "http.status_code"},
				&expression.Scalar{Value: "500"},
			}},
		},
		{
			name: "a level-qualified attribute and a typed constant",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.Reference{Name: "size", Level: expression.LevelResource, Attr: true},
				&expression.Scalar{Value: "500", Type: expression.ValueTypeInt},
			}},
		},
		{
			name: "a nested boolean tree with a list operand",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpNot, Args: []expression.Expression{
					&expression.Call{Op: expression.OpExists, Args: []expression.Expression{
						&expression.Reference{Name: "error"},
					}},
				}},
				&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
					&expression.Reference{Name: "http.status_code"},
					&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
				}},
			}},
		},
		{
			name: "a correlated match over the event collection",
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.Reference{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.Reference{Level: expression.LevelEvent, Name: expression.EventFieldName},
					&expression.Scalar{Value: "exception"},
				}},
			}},
		},
		{
			name: "a comparison of two references",
			filter: &expression.Call{Op: expression.OpNe, Args: []expression.Expression{
				&expression.Reference{Name: "enduser.id", Level: expression.LevelSpan, Attr: true},
				&expression.Reference{Name: "enduser.id", Level: expression.LevelResource, Attr: true},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.filter, mustToFilter(t, FromFilter(test.filter)))
		})
	}
}

// TestToFilter_Invalid pins that decoding validates, so a caller cannot hand a backend a filter
// that names something this build has no meaning for. Encoding stays total: FromFilter relays
// the same unknown operator, because what a filter may say is what the backend declares, not
// what the encoder recognizes.
func TestToFilter_Invalid(t *testing.T) {
	unknownOp := &expression.Call{Op: "json_extract", Args: []expression.Expression{
		&expression.Reference{Name: "input"},
		&expression.Scalar{Value: "guardrails[0].is_passed"},
	}}
	assert.Equal(t, "json_extract", FromFilter(unknownOp).GetOp())

	_, err := ToFilter(FromFilter(unknownOp))
	require.ErrorContains(t, err, `unknown filter operator "json_extract"`)
}

// TestToFilter_EmptyTerms covers what a client can put on the wire that the AST has no
// place for: an expression with no term set, and a call argument whose call is absent. Each is
// refused, rather than dropped to leave a filter that quietly asks something narrower.
func TestToFilter_EmptyTerms(t *testing.T) {
	tests := []struct {
		name  string
		proto *Call
	}{
		{
			name: "an argument with no term",
			proto: &Call{Op: "eq", Args: []*Expression{
				{Term: &Expression_Ref{Ref: &Reference{Name: "a"}}},
				{},
			}},
		},
		{
			name: "a call argument with no call",
			proto: &Call{Op: "and", Args: []*Expression{
				{Term: &Expression_Call{Call: nil}},
			}},
		},
		{
			name: "an empty term two calls deep",
			proto: &Call{Op: "and", Args: []*Expression{
				{Term: &Expression_Call{Call: &Call{Op: "eq", Args: []*Expression{{}}}}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ToFilter(test.proto)
			require.ErrorContains(t, err, "filter argument is empty")
		})
	}
}

// TestFromStorageExpression_UnknownTerm pins that an expression outside the four term types — only
// reachable as a nil interface, since the interface is closed — encodes as an empty term
// rather than panicking.
func TestFromStorageExpression_UnknownTerm(t *testing.T) {
	assert.Equal(t, &Expression{}, fromFilterExpression(nil))
}
