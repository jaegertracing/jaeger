// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eq(left, right Expression) *Call {
	return &Call{Op: OpEq, Args: []Expression{left, right}}
}

func TestValidateFilter_Accepts(t *testing.T) {
	tests := []struct {
		name   string
		filter *Call
	}{
		{
			name:   "unqualified attribute equality",
			filter: eq(&Reference{Name: "http.status_code"}, &Scalar{Value: "500"}),
		},
		{
			name:   "level-qualified attribute",
			filter: eq(&Reference{Name: "k8s.pod.name", Level: LevelResource, Attr: true}, &Scalar{Value: "cart-0"}),
		},
		{
			name:   "built-in field against a typed constant",
			filter: &Call{Op: OpGt, Args: []Expression{SpanDuration.Ref(), &Scalar{Value: "2s"}}},
		},
		{
			name:   "reference against reference",
			filter: eq(&Reference{Name: "enduser.id", Level: LevelSpan, Attr: true}, &Reference{Name: "enduser.id", Level: LevelResource, Attr: true}),
		},
		{
			name: "conjunction of two predicates",
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				eq(&Reference{Name: "b"}, &Scalar{Value: "2"}),
			}},
		},
		{
			name: "nested disjunction under a negation",
			filter: &Call{Op: OpNot, Args: []Expression{
				&Call{Op: OpOr, Args: []Expression{
					eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
					eq(&Reference{Name: "b"}, &Scalar{Value: "2"}),
				}},
			}},
		},
		{
			name: "set membership",
			filter: &Call{Op: OpIn, Args: []Expression{
				&Reference{Name: "http.status_code"},
				&List{Values: []string{"500", "503"}, Type: ValueTypeInt},
			}},
		},
		{
			name:   "existence of an attribute",
			filter: &Call{Op: OpExists, Args: []Expression{&Reference{Name: "error"}}},
		},
		{
			name: "correlated match over the event collection",
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelEvent},
				&Call{Op: OpAnd, Args: []Expression{
					eq(EventName.Ref(), &Scalar{Value: "exception"}),
					&Call{Op: OpGt, Args: []Expression{
						&Reference{Name: "timeSinceStart", Level: LevelEvent},
						&Scalar{Value: "50us"},
					}},
				}},
			}},
		},
		{
			name: "a call result as an operand",
			filter: eq(
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}}},
				&Scalar{Value: "true", Type: ValueTypeBool},
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter))
		})
	}
}

func TestValidateFilter_Rejects(t *testing.T) {
	tests := []struct {
		name        string
		filter      *Call
		expectedErr string
	}{
		{
			name:        "no filter",
			filter:      nil,
			expectedErr: "filter is empty",
		},
		{
			name:        "unknown operator",
			expectedErr: `unknown filter operator "matches"`,
			filter:      &Call{Op: "matches", Args: []Expression{&Reference{Name: "a"}, &Scalar{Value: "b"}}},
		},
		{
			name:        "conjunction of one",
			expectedErr: `operator "and" takes at least two arguments, got 1`,
			filter:      &Call{Op: OpAnd, Args: []Expression{eq(&Reference{Name: "a"}, &Scalar{Value: "1"})}},
		},
		{
			name:        "conjunction over a bare reference",
			expectedErr: `operator "and" takes predicates as arguments, got a reference`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				&Reference{Name: "a"},
				eq(&Reference{Name: "b"}, &Scalar{Value: "1"}),
			}},
		},
		{
			name:        "invalid predicate nested in a conjunction",
			expectedErr: `unknown filter level "pod"`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				eq(&Reference{Name: "b", Level: "pod"}, &Scalar{Value: "2"}),
			}},
		},
		{
			name:        "negation of two predicates",
			expectedErr: `operator "not" takes 1 argument(s), got 2`,
			filter: &Call{Op: OpNot, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				eq(&Reference{Name: "b"}, &Scalar{Value: "2"}),
			}},
		},
		{
			name:        "equality of one operand",
			expectedErr: `operator "eq" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpEq, Args: []Expression{&Reference{Name: "a"}}},
		},
		{
			name:        "existence of a constant",
			expectedErr: `operator "exists" takes a reference, got a constant`,
			filter:      &Call{Op: OpExists, Args: []Expression{&Scalar{Value: "a"}}},
		},
		{
			name:        "existence of an unnamed reference",
			expectedErr: "filter reference has no name",
			filter:      &Call{Op: OpExists, Args: []Expression{&Reference{Level: LevelSpan}}},
		},
		{
			name:        "membership of a constant rather than a list",
			expectedErr: `operator "in" takes a list as its second argument, got a constant`,
			filter:      &Call{Op: OpIn, Args: []Expression{&Reference{Name: "a"}, &Scalar{Value: "1"}}},
		},
		{
			name:        "list with an unknown type",
			expectedErr: `unknown filter value type "number"`,
			filter:      &Call{Op: OpNotIn, Args: []Expression{&Reference{Name: "a"}, &List{Values: []string{"1"}, Type: "number"}}},
		},
		{
			name:        "constant with an unknown type",
			expectedErr: `unknown filter value type "timestamp"`,
			filter:      eq(&Reference{Name: "a"}, &Scalar{Value: "1", Type: "timestamp"}),
		},
		{
			name:        "list compared with equality",
			expectedErr: "a list cannot be compared",
			filter:      eq(&Reference{Name: "a"}, &List{Values: []string{"1"}}),
		},
		{
			name:        "an argument with no term",
			expectedErr: "an empty term cannot be compared",
			filter:      &Call{Op: OpEq, Args: []Expression{&Reference{Name: "a"}, nil}},
		},
		{
			name:        "quantifier over a constant",
			expectedErr: `operator "some" takes a collection reference as its first argument, got a constant`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Scalar{Value: "event"},
				&Call{Op: OpExists, Args: []Expression{EventName.Ref()}},
			}},
		},
		{
			name:        "quantifier over the span",
			expectedErr: `operator "some" quantifies over "event" or "link", got level "span"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelSpan},
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}}},
			}},
		},
		{
			name:        "quantifier over a named event field",
			expectedErr: `operator "some" takes the whole collection, so its first argument must not name "name"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				EventName.Ref(),
				&Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}}},
			}},
		},
		{
			name:        "quantifier over a constant predicate",
			expectedErr: `operator "some" takes a predicate as its second argument, got a constant`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelEvent},
				&Scalar{Value: "true"},
			}},
		},
		{
			name:        "quantifier with an invalid predicate",
			expectedErr: `unknown filter operator "matches"`,
			filter: &Call{Op: OpSome, Args: []Expression{
				&Reference{Level: LevelLink},
				&Call{Op: "matches", Args: []Expression{&Reference{Name: "a"}}},
			}},
		},
		{
			name:        "quantifier of one argument",
			expectedErr: `operator "some" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpSome, Args: []Expression{&Reference{Level: LevelEvent}}},
		},
		{
			name:        "existence of two references",
			expectedErr: `operator "exists" takes 1 argument(s), got 2`,
			filter:      &Call{Op: OpExists, Args: []Expression{&Reference{Name: "a"}, &Reference{Name: "b"}}},
		},
		{
			name:        "membership without a set",
			expectedErr: `operator "in" takes 2 argument(s), got 1`,
			filter:      &Call{Op: OpIn, Args: []Expression{&Reference{Name: "a"}}},
		},
		{
			name:        "conjunction of a predicate and a list",
			expectedErr: `operator "and" takes predicates as arguments, got a list`,
			filter: &Call{Op: OpAnd, Args: []Expression{
				eq(&Reference{Name: "a"}, &Scalar{Value: "1"}),
				&List{Values: []string{"1"}},
			}},
		},
		{
			name:        "membership with an invalid left operand",
			expectedErr: `unknown filter level "pod"`,
			filter:      &Call{Op: OpIn, Args: []Expression{&Reference{Name: "a", Level: "pod"}, &List{Values: []string{"1"}}}},
		},
		{
			name:        "invalid nested call as an operand",
			expectedErr: `unknown filter operator "matches"`,
			filter:      eq(&Call{Op: "matches", Args: []Expression{&Reference{Name: "a"}}}, &Scalar{Value: "1"}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateFilter(test.filter)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}

// TestExpressionTerms pins which types are filter terms. The marker method is what closes
// the interface to these four, so a backend switching on the concrete type covers every
// case.
func TestExpressionTerms(t *testing.T) {
	tests := []struct {
		term Expression
		name string
	}{
		{&Reference{}, "a reference"},
		{&Scalar{}, "a constant"},
		{&List{}, "a list"},
		{&Call{}, "a predicate"},
	}
	for _, test := range tests {
		test.term.isExpression()
		assert.Equal(t, test.name, termName(test.term))
	}
	assert.Equal(t, "an empty term", termName(nil))
}

// TestField_Ref pins that a Field carries its level into the Reference it builds, which is
// what keeps a field name from being read at a level it does not belong to.
func TestField_Ref(t *testing.T) {
	assert.Equal(t, &Reference{Name: "duration", Level: LevelSpan}, SpanDuration.Ref())
	assert.Equal(t, &Reference{Name: "service", Level: LevelResource}, ResourceService.Ref())
	assert.Equal(t, &Reference{Name: "name", Level: LevelSpan}, SpanName.Ref())
	assert.Equal(t, &Reference{Name: "name", Level: LevelEvent}, EventName.Ref())
}

// TestReference_IsAttribute pins that the question is not the Attr bit alone: an unqualified
// reference is an attribute however Attr is set, because no built-in field has an unqualified
// form.
func TestReference_IsAttribute(t *testing.T) {
	assert.True(t, (&Reference{Name: "http.method"}).IsAttribute(), "unqualified, Attr unset")
	assert.True(t, (&Reference{Name: "http.method", Attr: true}).IsAttribute())
	assert.True(t, (&Reference{Name: "http.method", Level: LevelSpan, Attr: true}).IsAttribute())
	assert.False(t, SpanDuration.Ref().IsAttribute())
	assert.False(t, (&Reference{Level: LevelEvent}).IsAttribute(), "a collection is not an attribute")
}

// TestReference_IsField covers the three ways a reference can fail to be a given built-in
// field: a different name, the same name at another level, and an attribute that borrows the
// field's spelling.
func TestReference_IsField(t *testing.T) {
	assert.True(t, SpanDuration.Ref().IsField(SpanDuration))
	assert.False(t, SpanName.Ref().IsField(SpanDuration), "a different field of the same level")
	assert.False(t, EventName.Ref().IsField(SpanName), "the same name is a different field per level")
	assert.False(t, SpanName.Ref().IsField(EventName))

	attribute := &Reference{Name: "duration", Level: LevelSpan, Attr: true}
	assert.False(t, attribute.IsField(SpanDuration), "an attribute is never the built-in field")

	unqualified := &Reference{Name: "service"}
	assert.False(t, unqualified.IsField(ResourceService), "an unqualified reference is an attribute")
}

func TestFilterCapabilities_SupportsLevel(t *testing.T) {
	caps := FilterCapabilities{Levels: []Level{LevelSpan, LevelResource}}
	assert.True(t, caps.SupportsLevel(""), "an unqualified reference always reaches the reader")
	assert.True(t, caps.SupportsLevel(LevelSpan))
	assert.True(t, caps.SupportsLevel(LevelResource))
	assert.False(t, caps.SupportsLevel(LevelLink))
	assert.False(t, FilterCapabilities{}.SupportsLevel(LevelSpan))
}

// TestFilterCapabilities_SupportsOperator pins that the boolean combinators are declared
// like any other operator: a reader confined to the conjunctive subset lists OpAnd and omits
// OpOr and OpNot, and nothing is implicit — the zero value declares no operator at all.
func TestFilterCapabilities_SupportsOperator(t *testing.T) {
	flat := FilterCapabilities{Operators: []Operator{OpAnd, OpEq, OpGte}}
	assert.True(t, flat.SupportsOperator(OpAnd))
	assert.True(t, flat.SupportsOperator(OpEq))
	assert.True(t, flat.SupportsOperator(OpGte))
	assert.False(t, flat.SupportsOperator(OpRegex))
	assert.False(t, flat.SupportsOperator(OpOr))
	assert.False(t, flat.SupportsOperator(OpNot))

	full := FilterCapabilities{Operators: []Operator{OpAnd, OpOr, OpNot, OpEq}}
	assert.True(t, full.SupportsOperator(OpOr))
	assert.True(t, full.SupportsOperator(OpNot))

	assert.False(t, FilterCapabilities{}.SupportsOperator(OpAnd), "nothing is implicit")
	assert.False(t, FilterCapabilities{}.SupportsOperator(OpEq))
}
