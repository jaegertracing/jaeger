// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

func eq(left, right expression.Expression) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{left, right}}
}

func attr(key string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: key}
}

func TestValidateFilter_Accepts(t *testing.T) {
	tests := []struct {
		name   string
		filter *expression.Call
	}{
		{
			name:   "unqualified attribute equality",
			filter: eq(attr("http.status_code"), &expression.AnyValue{Value: "500"}),
		},
		{
			name:   "text ordered against text",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.StringValue{Value: "m"}}},
		},
		{
			name:   "an attribute ordered against a number, since storage decides its type",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{attr("size"), &expression.IntValue{Value: 500}}},
		},
		{
			name:   "a timestamp field against an instant",
			filter: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan}, &expression.TimestampValue{Value: time.Unix(0, 0).UTC()}}},
		},
		{
			name:   "level-qualified attribute",
			filter: eq(&expression.AttributeRef{Key: "k8s.pod.name", Level: expression.LevelResource}, &expression.StringValue{Value: "cart-0"}),
		},
		{
			name: "built-in field against a duration",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "2s"},
			}},
		},
		{
			name: "the same attribute key at two levels",
			filter: eq(
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelSpan},
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelResource},
			),
		},
		{
			name: "two attributes ordered against each other",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.AttributeRef{Key: "queue.depth.after"},
				&expression.AttributeRef{Key: "queue.depth.before"},
			}},
		},
		{
			name: "two built-in fields holding the same kind of value",
			filter: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.FieldRef{Name: expression.SpanFieldEndTime, Level: expression.LevelSpan},
			}},
		},
		{
			name:   "two constants, which asks nothing about the span but is still a comparison",
			filter: eq(&expression.IntValue{Value: 1}, &expression.IntValue{Value: 1}),
		},
		{
			name: "conjunction of two predicates",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eq(attr("a"), &expression.AnyValue{Value: "1"}),
				eq(attr("b"), &expression.AnyValue{Value: "2"}),
			}},
		},
		{
			name: "nested disjunction under a negation",
			filter: &expression.Call{Op: expression.OpNot, Args: []expression.Expression{
				&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
					eq(attr("a"), &expression.AnyValue{Value: "1"}),
					eq(attr("b"), &expression.AnyValue{Value: "2"}),
				}},
			}},
		},
		{
			name: "set membership",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				attr("http.status_code"),
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
		},
		{
			name:   "existence of an attribute",
			filter: &expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("error")}},
		},
		{
			name: "a regular expression over a field",
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan},
				&expression.AnyValue{Value: "GET .*"},
			}},
		},
		{
			name: "membership of a field holding one of a set of words in a list of strings",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
				&expression.List{Values: []string{"server", "client"}, Type: expression.ValueTypeString},
			}},
		},
		{
			name: "a comparison written with the constant on the left",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.AnyValue{Value: "2s"}, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
			}},
		},
		{
			name: "a regular expression over a field holding one of a set of words",
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
				&expression.StringValue{Value: "produ.*"},
			}},
		},
		{
			name: "a regular expression with a typed pattern",
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				attr("http.route"),
				&expression.StringValue{Value: "/api/.*"},
			}},
		},
		{
			name: "an ordered comparison of typed constants",
			filter: &expression.Call{Op: expression.OpGte, Args: []expression.Expression{
				attr("http.response.size"),
				&expression.IntValue{Value: 500},
			}},
		},
		{
			name: "correlated match over the event collection",
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
					eq(&expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}, &expression.StringValue{Value: "exception"}),
					&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
						&expression.FieldRef{Name: expression.EventFieldTimeSinceStart, Level: expression.LevelEvent},
						&expression.AnyValue{Value: "50us"},
					}},
				}},
			}},
		},
		{
			name: "a quantifier nested over the other collection",
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpSome, Args: []expression.Expression{
					&expression.NestedRef{Level: expression.LevelLink},
					&expression.Call{Op: expression.OpExists, Args: []expression.Expression{&expression.FieldRef{Name: expression.LinkFieldTraceID, Level: expression.LevelLink}}},
				}},
			}},
		},
		{
			name: "two quantifiers over the same level side by side",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpSome, Args: []expression.Expression{
					&expression.NestedRef{Level: expression.LevelEvent},
					eq(&expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}, &expression.StringValue{Value: "exception"}),
				}},
				&expression.Call{Op: expression.OpSome, Args: []expression.Expression{
					&expression.NestedRef{Level: expression.LevelEvent},
					eq(&expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}, &expression.StringValue{Value: "retry"}),
				}},
			}},
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
		filter      *expression.Call
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
			filter:      &expression.Call{Op: "matches", Args: []expression.Expression{attr("a"), &expression.AnyValue{Value: "b"}}},
		},
		{
			name:        "conjunction of one",
			expectedErr: `operator "and" takes at least two arguments, got 1`,
			filter:      &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{eq(attr("a"), &expression.AnyValue{Value: "1"})}},
		},
		{
			name:        "conjunction over a bare reference",
			expectedErr: `operator "and" takes predicates as arguments, got an attribute reference`,
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				attr("a"),
				eq(attr("b"), &expression.AnyValue{Value: "1"}),
			}},
		},
		{
			name:        "invalid predicate nested in a conjunction",
			expectedErr: `unknown filter level "pod"`,
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eq(attr("a"), &expression.AnyValue{Value: "1"}),
				eq(&expression.AttributeRef{Key: "b", Level: "pod"}, &expression.AnyValue{Value: "2"}),
			}},
		},
		{
			name:        "negation of two predicates",
			expectedErr: `operator "not" takes 1 argument(s), got 2`,
			filter: &expression.Call{Op: expression.OpNot, Args: []expression.Expression{
				eq(attr("a"), &expression.AnyValue{Value: "1"}),
				eq(attr("b"), &expression.AnyValue{Value: "2"}),
			}},
		},
		{
			name:        "equality of one operand",
			expectedErr: `operator "eq" takes 2 argument(s), got 1`,
			filter:      &expression.Call{Op: expression.OpEq, Args: []expression.Expression{attr("a")}},
		},
		{
			name:        "equality of constants of different kinds",
			expectedErr: `operator "eq" compares an integer constant against a string constant, which hold different kinds of value`,
			filter:      eq(&expression.IntValue{Value: 1}, &expression.StringValue{Value: "1"}),
		},
		{
			name:        "existence of a constant",
			expectedErr: `operator "exists" takes a reference, got an untyped constant`,
			filter:      &expression.Call{Op: expression.OpExists, Args: []expression.Expression{&expression.AnyValue{Value: "a"}}},
		},
		{
			name:        "existence of an attribute with no key",
			expectedErr: "attribute reference has no key",
			filter:      &expression.Call{Op: expression.OpExists, Args: []expression.Expression{&expression.AttributeRef{Level: expression.LevelSpan}}},
		},
		{
			name:        "existence of a field with no name",
			expectedErr: "field reference has no name",
			filter:      &expression.Call{Op: expression.OpExists, Args: []expression.Expression{&expression.FieldRef{Level: expression.LevelSpan}}},
		},
		{
			name:        "a field with no level",
			expectedErr: "field reference has no level, and a built-in field belongs to one",
			filter:      eq(&expression.FieldRef{Name: expression.SpanFieldDuration}, &expression.AnyValue{Value: "2s"}),
		},
		{
			name:        "a field at an unknown level",
			expectedErr: `unknown filter level "pod"`,
			filter:      eq(&expression.FieldRef{Name: expression.SpanFieldDuration, Level: "pod"}, &expression.AnyValue{Value: "2s"}),
		},
		{
			name:        "membership of a constant rather than a list",
			expectedErr: `operator "in" takes a list as its second argument, got an untyped constant`,
			filter:      &expression.Call{Op: expression.OpIn, Args: []expression.Expression{attr("a"), &expression.AnyValue{Value: "1"}}},
		},
		{
			name:        "membership in an empty list",
			expectedErr: `operator "in" takes a list with at least one element`,
			filter:      &expression.Call{Op: expression.OpIn, Args: []expression.Expression{attr("a"), &expression.List{}}},
		},
		{
			name:        "membership of a constant subject",
			expectedErr: `operator "in" takes a reference, got a string constant`,
			filter:      &expression.Call{Op: expression.OpIn, Args: []expression.Expression{&expression.StringValue{Value: "a"}, &expression.List{Values: []string{"1"}}}},
		},
		{
			name:        "list with an unknown type",
			expectedErr: `unknown filter value type "number"`,
			filter:      &expression.Call{Op: expression.OpNotIn, Args: []expression.Expression{attr("a"), &expression.List{Values: []string{"1"}, Type: "number"}}},
		},
		{
			name:        "list compared with equality",
			expectedErr: `operator "eq" compares a reference or a constant, got a list`,
			filter:      eq(attr("a"), &expression.List{Values: []string{"1"}}),
		},
		{
			name:        "a call result as an operand",
			expectedErr: `operator "eq" compares a reference or a constant, got a predicate`,
			filter: eq(
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("a")}},
				&expression.BoolValue{Value: true},
			),
		},
		{
			name:        "a call result as the subject of membership",
			expectedErr: `operator "in" takes a reference, got a predicate`,
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("a")}},
				&expression.List{Values: []string{"true"}, Type: expression.ValueTypeBool},
			}},
		},
		{
			name:        "an ordered comparison of a word-valued field",
			expectedErr: `operator "gt" has no ordering for span.kind`,
			filter:      &expression.Call{Op: expression.OpGt, Args: []expression.Expression{&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan}, &expression.AnyValue{Value: "server"}}},
		},
		{
			name:        "an ordered comparison of a boolean",
			expectedErr: `operator "gt" has no ordering for a boolean constant`,
			filter:      &expression.Call{Op: expression.OpGt, Args: []expression.Expression{attr("ok"), &expression.BoolValue{Value: true}}},
		},
		{
			name:        "a duration field against a number",
			expectedErr: `operator "gt" compares span.duration against an integer constant, which hold different kinds of value`,
			filter:      &expression.Call{Op: expression.OpGt, Args: []expression.Expression{&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, &expression.IntValue{Value: 42}}},
		},
		{
			name:        "an argument with no term",
			expectedErr: `operator "eq" compares a reference or a constant, got an empty term`,
			filter:      &expression.Call{Op: expression.OpEq, Args: []expression.Expression{attr("a"), nil}},
		},
		{
			name:        "an ordered comparison against text",
			expectedErr: `operator "gt" compares span.duration against a string constant, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.StringValue{Value: "2s"},
			}},
		},
		{
			name:        "an ordered comparison of constants of different kinds",
			expectedErr: `operator "gte" compares an integer constant against a boolean constant, which hold different kinds of value`,
			filter:      &expression.Call{Op: expression.OpGte, Args: []expression.Expression{&expression.IntValue{Value: 1}, &expression.BoolValue{Value: true}}},
		},
		{
			name:        "an ordered comparison against a boolean",
			expectedErr: `operator "lte" has no ordering for a boolean constant`,
			filter:      &expression.Call{Op: expression.OpLte, Args: []expression.Expression{attr("a"), &expression.BoolValue{Value: true}}},
		},
		{
			name:        "two built-in fields holding different kinds of value",
			expectedErr: `operator "eq" compares span.duration against span.name, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan},
			}},
		},
		{
			name:        "an instant ordered against a duration",
			expectedErr: `operator "gt" compares span.startTime against span.duration, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
			}},
		},
		{
			name:        "an integer against a duration field",
			expectedErr: `operator "eq" compares span.duration against an integer constant, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, &expression.IntValue{Value: 2},
			}},
		},
		{
			name:        "a timestamp against a duration field",
			expectedErr: `operator "gt" compares span.duration against a timestamp constant, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.TimestampValue{Value: time.Unix(0, 0).UTC()},
			}},
		},
		{
			name:        "a duration against a text field",
			expectedErr: `operator "eq" compares span.name against a duration constant, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.DurationValue{Value: time.Second},
			}},
		},
		{
			name:        "a number against a field holding one of a set of words",
			expectedErr: `operator "eq" compares span.kind against an integer constant, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan}, &expression.IntValue{Value: 1},
			}},
		},
		{
			name:        "a boolean against a text field",
			expectedErr: `operator "eq" compares span.name against a boolean constant, which hold different kinds of value`,
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.BoolValue{Value: true},
			}},
		},
		{
			name:        "a duration constant compared against an attribute",
			expectedErr: `operator "gt" compares a duration constant against an attribute, and the wire has no duration type`,
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				attr("latency"), &expression.DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name:        "a timestamp constant compared against an attribute",
			expectedErr: `operator "eq" compares a timestamp constant against an attribute, and the wire has no timestamp type`,
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				attr("deadline"), &expression.TimestampValue{Value: time.Unix(0, 0).UTC()},
			}},
		},
		{
			name:        "an anchored regular expression",
			expectedErr: `operator "regex" matches anywhere in the value, so a pattern cannot anchor itself`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.StringValue{Value: "^GET"},
			}},
		},
		{
			name:        "a regular expression that will not compile",
			expectedErr: `operator "regex" takes a pattern in RE2 syntax`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.StringValue{Value: "GET ("},
			}},
		},
		{
			name:        "a regular expression with a word boundary",
			expectedErr: `operator "regex" takes a pattern without word boundaries`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.StringValue{Value: `\bGET`},
			}},
		},
		{
			name:        "a regular expression with a lazy quantifier",
			expectedErr: `operator "regex" asks whether the value matches, so a quantifier cannot be lazy`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.StringValue{Value: "GET.*?/"},
			}},
		},
		{
			name:        "a regular expression asking to fold case",
			expectedErr: `operator "regex" matches case-sensitively, so a pattern cannot fold case`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, &expression.StringValue{Value: "(?i)get"},
			}},
		},
		{
			name:        "a regular expression over a duration field",
			expectedErr: `operator "regex" matches text, and span.duration holds a duration`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, &expression.StringValue{Value: "2s"},
			}},
		},
		{
			name:        "a regular expression over a timestamp field",
			expectedErr: `operator "regex" matches text, and span.startTime holds a timestamp`,
			filter: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan}, &expression.StringValue{Value: "2026-.*"},
			}},
		},
		{
			name:        "a regular expression over a numeric pattern",
			expectedErr: `operator "regex" takes a constant string as its pattern, got an integer constant`,
			filter:      &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{attr("a"), &expression.IntValue{Value: 1}}},
		},
		{
			name:        "a regular expression matched against a reference",
			expectedErr: `operator "regex" takes a constant string as its pattern, got an attribute reference`,
			filter:      &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{attr("a"), attr("b")}},
		},
		{
			name:        "a regular expression of one argument",
			expectedErr: `operator "regex" takes 2 argument(s), got 1`,
			filter:      &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{attr("a")}},
		},
		{
			name:        "a regular expression over a constant",
			expectedErr: `operator "regex" takes a reference, got a string constant`,
			filter:      &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{&expression.StringValue{Value: "a"}, &expression.StringValue{Value: "b"}}},
		},
		{
			name:        "quantifier over a constant",
			expectedErr: `operator "some" takes a collection reference as its first argument, got an untyped constant`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.AnyValue{Value: "event"},
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{&expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}}},
			}},
		},
		{
			name:        "quantifier over an attribute",
			expectedErr: `operator "some" takes a collection reference as its first argument, got an attribute reference`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.AttributeRef{Key: "a", Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier over the span",
			expectedErr: `operator "some" quantifies over "event" or "link", got level "span"`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelSpan},
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier over a level-less collection",
			expectedErr: `operator "some" quantifies over "event" or "link", got level ""`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{},
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier over a constant predicate",
			expectedErr: `operator "some" takes a predicate as its second argument, got an untyped constant`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelEvent},
				&expression.AnyValue{Value: "true"},
			}},
		},
		{
			name:        "quantifier with an invalid predicate",
			expectedErr: `unknown filter operator "matches"`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelLink},
				&expression.Call{Op: "matches", Args: []expression.Expression{attr("a")}},
			}},
		},
		{
			name:        "quantifier of one argument",
			expectedErr: `operator "some" takes 2 argument(s), got 1`,
			filter:      &expression.Call{Op: expression.OpSome, Args: []expression.Expression{&expression.NestedRef{Level: expression.LevelEvent}}},
		},
		{
			name:        "existence of two references",
			expectedErr: `operator "exists" takes 1 argument(s), got 2`,
			filter:      &expression.Call{Op: expression.OpExists, Args: []expression.Expression{attr("a"), attr("b")}},
		},
		{
			name:        "membership without a set",
			expectedErr: `operator "in" takes 2 argument(s), got 1`,
			filter:      &expression.Call{Op: expression.OpIn, Args: []expression.Expression{attr("a")}},
		},
		{
			name:        "conjunction of a predicate and a list",
			expectedErr: `operator "and" takes predicates as arguments, got a list`,
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eq(attr("a"), &expression.AnyValue{Value: "1"}),
				&expression.List{Values: []string{"1"}},
			}},
		},
		{
			name:        "membership with an invalid left operand",
			expectedErr: `unknown filter level "pod"`,
			filter:      &expression.Call{Op: expression.OpIn, Args: []expression.Expression{&expression.AttributeRef{Key: "a", Level: "pod"}, &expression.List{Values: []string{"1"}}}},
		},
		{
			name:        "invalid nested call as an operand",
			expectedErr: `operator "eq" compares a reference or a constant, got a predicate`,
			filter:      eq(&expression.Call{Op: "matches", Args: []expression.Expression{attr("a")}}, &expression.AnyValue{Value: "1"}),
		},
		{
			name:        "invalid nested call as the subject of membership",
			expectedErr: `operator "in" takes a reference, got a predicate`,
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.Call{Op: "matches", Args: []expression.Expression{attr("a")}},
				&expression.List{Values: []string{"1"}},
			}},
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

// TestValidateFilter_RejectsANestedRefOutsideSome pins that the nested collection is readable
// only by the quantifier. Anywhere else it is many values where one is expected.
func TestValidateFilter_RejectsANestedRefOutsideSome(t *testing.T) {
	expected := `a collection reference is only the first argument of "some"`
	collection := &expression.NestedRef{Level: expression.LevelEvent}

	tests := map[string]*expression.Call{
		"compared against a constant": eq(collection, &expression.AnyValue{Value: "1"}),
		"under exists":                {Op: expression.OpExists, Args: []expression.Expression{collection}},
		"as the subject of membership": {Op: expression.OpIn, Args: []expression.Expression{
			collection, &expression.List{Values: []string{"1"}},
		}},
		"as the predicate of a quantifier": {Op: expression.OpSome, Args: []expression.Expression{
			&expression.NestedRef{Level: expression.LevelLink},
			&expression.Call{Op: expression.OpExists, Args: []expression.Expression{collection}},
		}},
	}
	for name, filter := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, ValidateFilter(filter), expected)
		})
	}
}

// TestValidateFilter_RejectsANestedSomeOverTheSameLevel pins RFC 0005 §5.5 rule 4. The inner
// quantifier would have to either shadow the outer element or reach back to it, and the version
// that answers that question is not this one.
func TestValidateFilter_RejectsANestedSomeOverTheSameLevel(t *testing.T) {
	inner := func(level expression.Level) *expression.Call {
		return &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
			&expression.NestedRef{Level: level},
			&expression.Call{Op: expression.OpExists, Args: []expression.Expression{&expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}}},
		}}
	}
	directly := &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
		&expression.NestedRef{Level: expression.LevelEvent},
		inner(expression.LevelEvent),
	}}
	require.ErrorContains(t, ValidateFilter(directly),
		`operator "some" is already quantifying over "event", and this version does not define what a nested one would bind`)

	// However deep the inner one sits, and whichever level is doubled up.
	deeper := &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
		&expression.NestedRef{Level: expression.LevelLink},
		&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
			eq(&expression.FieldRef{Name: expression.LinkFieldTraceID, Level: expression.LevelLink}, &expression.StringValue{Value: "abc"}),
			&expression.Call{Op: expression.OpNot, Args: []expression.Expression{inner(expression.LevelLink)}},
		}},
	}}
	require.ErrorContains(t, ValidateFilter(deeper), `already quantifying over "link"`)
}

// TestValidateFilter_RejectsUnknownField pins that naming a field this API does not define is
// refused, and that the message says how to ask for an attribute of that name instead.
func TestValidateFilter_RejectsUnknownField(t *testing.T) {
	err := ValidateFilter(eq(&expression.FieldRef{Level: expression.LevelSpan, Name: "durtion"}, &expression.AnyValue{Value: "2s"}))
	require.ErrorContains(t, err, `unknown built-in field "durtion" at the "span" level`)
	require.ErrorContains(t, err, "name an attribute to match a tag of that name instead")

	// The same name as an attribute is fine, because an attribute key is arbitrary.
	require.NoError(t, ValidateFilter(
		eq(&expression.AttributeRef{Level: expression.LevelSpan, Key: "durtion"}, &expression.AnyValue{Value: "2s"}),
	))

	// A field of the wrong level is refused too.
	require.ErrorContains(t,
		ValidateFilter(eq(&expression.FieldRef{Level: expression.LevelResource, Name: expression.SpanFieldDuration}, &expression.AnyValue{Value: "2s"})),
		`unknown built-in field "duration" at the "resource" level`)
}

// The tests below are the systematic half. The tables above enumerate cases someone thought of;
// these walk the vocabulary itself, so a constant added without a matching case in the
// validator fails here rather than passing unnoticed until a caller sends it.

// TestDomainOfAnUndeclaredType pins what an unnamed type says about a value: nothing. Validation
// refuses an undefined field and an unknown value type before either domain is asked about, so
// this is the one place the zero value of each is exercised.
func TestDomainOfAnUndeclaredType(t *testing.T) {
	assert.Equal(t, domainUnknown, domainOfFieldType(""))
	assert.Equal(t, domainUnknown, domainOfValueType(""))
}

// TestValidateFilter_HandlesEveryOperator pins that each declared operator has a case in
// validateCall. Without this, adding an operator constant and forgetting the case would report
// it to callers as unknown — the one answer that is certainly wrong, since the API defines it.
func TestValidateFilter_HandlesEveryOperator(t *testing.T) {
	for _, op := range expression.Operators() {
		t.Run(string(op), func(t *testing.T) {
			// Deliberately the wrong arguments: what matters is only that the operator is
			// recognised, so any complaint except "unknown" means it has a case.
			err := ValidateFilter(&expression.Call{Op: op})
			if err != nil {
				assert.NotContains(t, err.Error(), "unknown filter operator",
					"operator %q is declared but validateCall has no case for it", op)
			}
		})
	}
}

// TestValidateFilter_AcceptsEveryLevel pins that an attribute may name any declared level.
func TestValidateFilter_AcceptsEveryLevel(t *testing.T) {
	for _, level := range expression.Levels() {
		t.Run(string(level), func(t *testing.T) {
			ref := &expression.AttributeRef{Level: level, Key: "a"}
			require.NoError(t, ValidateFilter(eq(ref, &expression.AnyValue{Value: "1"})))
		})
	}
	require.NoError(t, ValidateFilter(eq(attr("a"), &expression.AnyValue{Value: "1"})),
		"an empty level is the unqualified attribute and is always allowed")
}

// TestValidateFilter_AcceptsEveryValueType pins that a list may declare any of the defined types.
// Declaring none is accepted only where the field opposite the list declares one instead, which is
// what the two subjects here distinguish.
func TestValidateFilter_AcceptsEveryValueType(t *testing.T) {
	for _, vt := range expression.ValueTypes() {
		t.Run(string(vt), func(t *testing.T) {
			in := &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				attr("a"),
				&expression.List{Values: []string{"1"}, Type: vt},
			}}
			require.NoError(t, ValidateFilter(in))
		})
	}

	t.Run("no type, against a built-in field", func(t *testing.T) {
		in := &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
			spanField(expression.SpanFieldDuration),
			&expression.List{Values: []string{"2s"}},
		}}
		require.NoError(t, ValidateFilter(in))
	})

	// An attribute is under no type constraint, so a list beside one needs no declared type
	// either — the same concession `eq` against an attribute has always made for a scalar.
	t.Run("no type, against an attribute", func(t *testing.T) {
		in := &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
			attr("a"),
			&expression.List{Values: []string{"1"}},
		}}
		require.NoError(t, ValidateFilter(in))
	})
}

// TestValidateFilter_AcceptsEveryConstant pins that every constant node is comparable against
// something, since each one is what a wire hint or a resolution can produce. A duration and an
// instant are comparable only against a built-in field that declares that type (§5.4), which is why
// the reference this compares against depends on the constant.
func TestValidateFilter_AcceptsEveryConstant(t *testing.T) {
	for _, test := range allConstants {
		t.Run(test.name, func(t *testing.T) {
			var reference expression.Expression = attr("a")
			switch test.term.(type) {
			case *expression.DurationValue:
				reference = spanField(expression.SpanFieldDuration)
			case *expression.TimestampValue:
				reference = spanField(expression.SpanFieldStartTime)
			default:
				// Every other constant is comparable against an attribute, which declares
				// no type of its own.
			}
			require.NoError(t, ValidateFilter(eq(reference, test.term)))
		})
	}
}

// TestValidateFilter_AcceptsEveryField pins that the field enumeration and the validator agree:
// every field Fields() offers is one a query may actually name at that level.
func TestValidateFilter_AcceptsEveryField(t *testing.T) {
	for _, f := range expression.Fields() {
		t.Run(string(f.Level)+"."+f.Name, func(t *testing.T) {
			ref := &expression.FieldRef{Level: f.Level, Name: f.Name}
			require.NoError(t, ValidateFilter(eq(ref, &expression.AnyValue{Value: "x"})))
		})
	}
}

// TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth pins that validation recurses down every
// path a node can hide in, not just the root. Each case buries the same bad reference — an
// unknown level — somewhere different.
func TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth(t *testing.T) {
	bad := &expression.AttributeRef{Level: "nonesuch", Key: "a"}
	good := eq(attr("ok"), &expression.AnyValue{Value: "1"})

	tests := map[string]*expression.Call{
		"under and":               {Op: expression.OpAnd, Args: []expression.Expression{good, eq(bad, &expression.AnyValue{Value: "1"})}},
		"under or":                {Op: expression.OpOr, Args: []expression.Expression{good, eq(bad, &expression.AnyValue{Value: "1"})}},
		"under not":               {Op: expression.OpNot, Args: []expression.Expression{eq(bad, &expression.AnyValue{Value: "1"})}},
		"as a comparison operand": eq(bad, &expression.AnyValue{Value: "1"}),
		"as the right operand":    eq(attr("ok"), bad),
		"as the subject of in":    {Op: expression.OpIn, Args: []expression.Expression{bad, &expression.List{Values: []string{"1"}}}},
		"as the subject of regex": {Op: expression.OpRegex, Args: []expression.Expression{bad, &expression.StringValue{Value: "1"}}},
		"under exists":            {Op: expression.OpExists, Args: []expression.Expression{bad}},
		"inside a some predicate": {Op: expression.OpSome, Args: []expression.Expression{&expression.NestedRef{Level: expression.LevelEvent}, eq(bad, &expression.AnyValue{Value: "1"})}},
		"two conjunctions deep":   {Op: expression.OpAnd, Args: []expression.Expression{good, &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{good, eq(bad, &expression.AnyValue{Value: "1"})}}}},
	}
	for name, filter := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, ValidateFilter(filter), `unknown filter level "nonesuch"`)
		})
	}
}

// TestValidateFilter_NeverPanics pins that validation answers for any tree at all, however
// malformed. It reaches trees a caller should not be able to build but a decoder can produce —
// a nil argument, a nil interface, a term where a predicate belongs — and asserts only that it
// returns. A validator that panicked would take the process down on a hostile request.
func TestValidateFilter_NeverPanics(t *testing.T) {
	trees := map[string]*expression.Call{
		"no operator":                      {},
		"no arguments":                     {Op: expression.OpEq},
		"a nil argument":                   {Op: expression.OpEq, Args: []expression.Expression{nil, nil}},
		"a nil attribute reference":        {Op: expression.OpEq, Args: []expression.Expression{(*expression.AttributeRef)(nil), &expression.AnyValue{Value: "1"}}},
		"a nil field reference":            {Op: expression.OpEq, Args: []expression.Expression{(*expression.FieldRef)(nil), &expression.AnyValue{Value: "1"}}},
		"a nil collection reference":       {Op: expression.OpSome, Args: []expression.Expression{(*expression.NestedRef)(nil), &expression.Call{Op: expression.OpExists}}},
		"a nil constant":                   {Op: expression.OpEq, Args: []expression.Expression{attr("a"), (*expression.AnyValue)(nil)}},
		"a nil list":                       {Op: expression.OpIn, Args: []expression.Expression{attr("a"), (*expression.List)(nil)}},
		"a nil nested call":                {Op: expression.OpAnd, Args: []expression.Expression{(*expression.Call)(nil), (*expression.Call)(nil)}},
		"a list where a predicate belongs": {Op: expression.OpAnd, Args: []expression.Expression{&expression.List{}, &expression.List{}}},
		"a call as its own argument":       {Op: expression.OpNot, Args: []expression.Expression{&expression.Call{Op: expression.OpNot, Args: nil}}},
		"some over nothing":                {Op: expression.OpSome, Args: []expression.Expression{nil, nil}},
	}
	for name, filter := range trees {
		t.Run(name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { err = ValidateFilter(filter) })
			require.Error(t, err, "a malformed tree is refused, not merely survived")
		})
	}
	assert.Error(t, ValidateFilter(nil))
}

// nestedTo builds a filter nesting calls the given number of levels deep, counting the outermost.
func nestedTo(depth int) *expression.Call {
	filter := eq(attr("a"), &expression.AnyValue{Value: "1"})
	for range depth - 1 {
		filter = &expression.Call{Op: expression.OpNot, Args: []expression.Expression{filter}}
	}
	return filter
}

// TestValidateFilter_BoundsNesting pins the depth a filter may nest to. Walking the tree is
// recursive at every layer that reads a filter, so the bound is what keeps a tree nobody could walk
// from being accepted here.
func TestValidateFilter_BoundsNesting(t *testing.T) {
	require.NoError(t, ValidateFilter(nestedTo(expression.MaxNestingDepth)))
	require.ErrorIs(t, ValidateFilter(nestedTo(expression.MaxNestingDepth+1)), expression.ErrTooDeeplyNested)

	// The same bound answers a quantifier's nesting, which recurses through its own predicate.
	quantified := &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
		&expression.NestedRef{Level: expression.LevelEvent}, nestedTo(expression.MaxNestingDepth),
	}}
	require.ErrorIs(t, ValidateFilter(quantified), expression.ErrTooDeeplyNested)
}

// TestValidateFilter_RefusesAFilterThatContainsItself is the reason the bound exists rather than a
// cycle check: the AST is built from pointers, so a caller can hand a call itself as one of its own
// arguments, and the walk would follow it until the stack ran out. Being too deep is the answer.
func TestValidateFilter_RefusesAFilterThatContainsItself(t *testing.T) {
	cycle := &expression.Call{Op: expression.OpNot}
	cycle.Args = []expression.Expression{cycle}
	require.ErrorIs(t, ValidateFilter(cycle), expression.ErrTooDeeplyNested)

	outer := &expression.Call{Op: expression.OpAnd}
	inner := &expression.Call{Op: expression.OpNot, Args: []expression.Expression{outer}}
	outer.Args = []expression.Expression{inner, eq(attr("a"), &expression.AnyValue{Value: "1"})}
	require.ErrorIs(t, ValidateFilter(outer), expression.ErrTooDeeplyNested)
}

// TestValidateFilter_RejectsATypedNilConstant walks every constant type through both positions a
// constant can occupy. A nil pointer of one reads through the Expression interface as a term of
// that type while holding no value, so validation has to refuse it rather than leave a later stage
// to dereference it.
func TestValidateFilter_RejectsATypedNilConstant(t *testing.T) {
	constants := map[string]expression.Expression{
		"an untyped constant":       (*expression.AnyValue)(nil),
		"a string constant":         (*expression.StringValue)(nil),
		"an integer constant":       (*expression.IntValue)(nil),
		"a floating-point constant": (*expression.DoubleValue)(nil),
		"a boolean constant":        (*expression.BoolValue)(nil),
		"a duration constant":       (*expression.DurationValue)(nil),
		"a timestamp constant":      (*expression.TimestampValue)(nil),
	}
	positions := map[string]func(expression.Expression) *expression.Call{
		"compared against an attribute": func(c expression.Expression) *expression.Call {
			return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{attr("a"), c}}
		},
		"compared against a built-in field": func(c expression.Expression) *expression.Call {
			return &expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), c}}
		},
		"written on the left of a comparison": func(c expression.Expression) *expression.Call {
			return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{c, spanField(expression.SpanFieldName)}}
		},
		"as a regular expression": func(c expression.Expression) *expression.Call {
			return &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{spanField(expression.SpanFieldName), c}}
		},
		"as a list element's neighbour": func(c expression.Expression) *expression.Call {
			return &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpIn, Args: []expression.Expression{attr("a"), &expression.List{Values: []string{"1"}, Type: expression.ValueTypeInt}}},
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{attr("b"), c}},
			}}
		},
	}
	for name, constant := range constants {
		for where, build := range positions {
			t.Run(name+" "+where, func(t *testing.T) {
				filter := build(constant)
				require.ErrorContains(t, ValidateFilter(filter), "an empty term")
				assert.NotPanics(t, func() { _, _ = ResolveFilterConstants(filter) },
					"resolution answers for a tree validation refused")
			})
		}
	}
}

// allConstants is every constant term, as the pointer a tree is built from, paired with the name
// an error message gives it. TestValidateFilter_AcceptsEveryConstant walks it to check that each
// one is comparable against something. The jaeger-idl package's own tests are what pin this list
// against the full set of term types.
var allConstants = []struct {
	term expression.Expression
	name string
}{
	{&expression.AnyValue{}, "an untyped constant"},
	{&expression.StringValue{}, "a string constant"},
	{&expression.IntValue{}, "an integer constant"},
	{&expression.DoubleValue{}, "a floating-point constant"},
	{&expression.BoolValue{}, "a boolean constant"},
	{&expression.DurationValue{}, "a duration constant"},
	{&expression.TimestampValue{}, "a timestamp constant"},
}

// unknownTerm is a term the AST does not define. jaeger-idl closes Expression with an unexported
// marker, so no type here can implement it outright; embedding a term promotes the marker and
// gives this one a concrete type none of the switches name. It exists so the branches answering
// for a term added in a later jaeger-idl release are exercised rather than trusted.
type unknownTerm struct {
	*expression.AttributeRef
}

func TestValidateFilter_RefusesAnUnknownTerm(t *testing.T) {
	var term expression.Expression = &unknownTerm{}
	err := ValidateFilter(eq(attr("a"), term))
	require.ErrorContains(t, err, "an unknown term")
}
