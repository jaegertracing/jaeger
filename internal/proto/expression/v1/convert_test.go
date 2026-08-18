// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// mustFromProto decodes a filter that is expected to be well formed.
func mustFromProto(t *testing.T, filter *Call) *expression.Call {
	t.Helper()
	got, err := FromProto(filter)
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
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.AnyValue{Value: "500"},
			}},
		},
		{
			name: "a level-qualified attribute and an integer constant",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.AttributeRef{Key: "size", Level: expression.LevelResource},
				&expression.IntValue{Value: 500},
			}},
		},
		{
			name: "a built-in field and a floating-point constant",
			filter: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.AttributeRef{Key: "sampler.param", Level: expression.LevelSpan},
				&expression.DoubleValue{Value: 0.001},
			}},
		},
		{
			name: "a text constant and a boolean one",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
					&expression.StringValue{Value: "GET /health"},
				}},
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.AttributeRef{Key: "error"},
					&expression.BoolValue{Value: true},
				}},
			}},
		},
		{
			name: "a nested boolean tree with a list operand",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpNot, Args: []expression.Expression{
					&expression.Call{Op: expression.OpExists, Args: []expression.Expression{
						&expression.AttributeRef{Key: "error"},
					}},
				}},
				&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
					&expression.AttributeRef{Key: "http.status_code"},
					&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
				}},
			}},
		},
		{
			name: "a correlated match over the event collection",
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelEvent, Name: expression.EventFieldName},
					&expression.AnyValue{Value: "exception"},
				}},
			}},
		},
		{
			name: "a comparison of two references",
			filter: &expression.Call{Op: expression.OpNe, Args: []expression.Expression{
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelSpan},
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelResource},
			}},
		},
		{
			// The value sets are open, and neither direction judges the tree, so an operator this
			// build does not define survives the trip. Whether a backend may be asked it is a
			// separate question, answered by its declared capabilities.
			name: "an operator this build does not define",
			filter: &expression.Call{Op: "json_extract", Args: []expression.Expression{
				&expression.AttributeRef{Key: "input"},
				&expression.AnyValue{Value: "guardrails[0].is_passed"},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.filter, mustFromProto(t, mustToProto(t, test.filter)))
		})
	}
}

// TestTimeConstantsTravelUnhinted covers the two constants the wire has no type for. They are
// written in the syntax the field they are compared against is written in and come back untyped,
// which is the constant expression.ResolveConstants reads as that field's type again.
func TestTimeConstantsTravelUnhinted(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		built   expression.Expression
		encoded *Scalar
		decoded expression.Expression
	}{
		{
			name:    "a duration",
			field:   expression.SpanFieldDuration,
			built:   &expression.DurationValue{Value: 2*time.Second + 500*time.Millisecond},
			encoded: &Scalar{Value: "2.5s"},
			decoded: &expression.AnyValue{Value: "2.5s"},
		},
		{
			name:    "an instant",
			field:   expression.SpanFieldStartTime,
			built:   &expression.TimestampValue{Value: time.Date(2026, 8, 16, 18, 56, 20, 123456789, time.UTC)},
			encoded: &Scalar{Value: "2026-08-16T18:56:20.123456789Z"},
			decoded: &expression.AnyValue{Value: "2026-08-16T18:56:20.123456789Z"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := &expression.FieldRef{Level: expression.LevelSpan, Name: test.field}
			filter := &expression.Call{Op: expression.OpGt, Args: []expression.Expression{ref, test.built}}

			encoded := mustToProto(t, filter)
			assert.Equal(t, test.encoded, encoded.GetArgs()[1].GetScalar())

			decoded := mustFromProto(t, encoded)
			assert.Equal(t, test.decoded, decoded.Args[1])

			resolved, err := expression.ResolveConstants(decoded)
			require.NoError(t, err)
			assert.Equal(t, filter, resolved)
		})
	}
}

// TestFromProto_EmptyTerms covers what a client can put on the wire that the AST has no
// place for: an expression with no term set, and a call argument whose call is absent. Each is
// refused, rather than dropped to leave a filter that quietly asks something narrower.
func TestFromProto_EmptyTerms(t *testing.T) {
	tests := []struct {
		name  string
		proto *Call
	}{
		{
			name: "an argument with no term",
			proto: &Call{Op: "eq", Args: []*Expression{
				{Term: &Expression_Attr{Attr: &AttributeReference{Key: "a"}}},
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
			_, err := FromProto(test.proto)
			require.ErrorContains(t, err, "filter argument is empty")
		})
	}
}

// TestFromProto_ConstantNotOfItsDeclaredType covers a constant the AST has no node for, because
// its spelling is not the type it says it is. Reading it as untyped instead would drop the
// narrowing the caller asked for and answer a wider query than the one sent (RFC 0005 §5.4).
func TestFromProto_ConstantNotOfItsDeclaredType(t *testing.T) {
	tests := []struct {
		name    string
		scalar  *Scalar
		wantErr string
	}{
		{
			name:    "not an integer",
			scalar:  &Scalar{Value: "500ms", Type: "int"},
			wantErr: `filter constant "500ms" is not the "int" it declares`,
		},
		{
			name:    "not a number",
			scalar:  &Scalar{Value: "banana", Type: "double"},
			wantErr: `filter constant "banana" is not the "double" it declares`,
		},
		{
			name:    "not a boolean",
			scalar:  &Scalar{Value: "yes please", Type: "bool"},
			wantErr: `filter constant "yes please" is not the "bool" it declares`,
		},
		{
			name:    "a type this build does not define",
			scalar:  &Scalar{Value: "500", Type: "int32"},
			wantErr: `filter constant "500" declares the unknown type "int32"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FromProto(&Call{Op: "eq", Args: []*Expression{
				{Term: &Expression_Attr{Attr: &AttributeReference{Key: "a"}}},
				{Term: &Expression_Scalar{Scalar: test.scalar}},
			}})
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

// TestToProto_RefusesATermItCannotWrite covers the terms with no wire form: a missing one, and a
// nil pointer of a term type. An empty oneof arm decodes as an argument with no term, so writing
// one would hand the receiver a different filter instead of an error.
func TestToProto_RefusesATermItCannotWrite(t *testing.T) {
	terms := map[string]expression.Expression{
		"no term at all":            nil,
		"a nil attribute reference": (*expression.AttributeRef)(nil),
		"a nil field reference":     (*expression.FieldRef)(nil),
		"a nil collection":          (*expression.NestedRef)(nil),
		"a nil list":                (*expression.List)(nil),
		"a nil call":                (*expression.Call)(nil),
	}
	for name, term := range terms {
		t.Run(name, func(t *testing.T) {
			encoded, err := fromFilterExpression(term)
			require.ErrorIs(t, err, ErrTermNotEncodable)
			assert.Nil(t, encoded)
		})
	}

	_, err := ToProto(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: "a"}, nil,
	}})
	require.ErrorIs(t, err, ErrTermNotEncodable)
}

// mustToProto encodes a filter the test means to be encodable.
func mustToProto(t *testing.T, filter *expression.Call) *Call {
	t.Helper()
	encoded, err := ToProto(filter)
	require.NoError(t, err)
	return encoded
}
