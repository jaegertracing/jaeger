// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
)

var p builder.Predicate

func eq(left, right expression.Expression) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{left, right}}
}

func attr(key string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: key}
}

// words builds the list a text-search operator takes, with no declared type.
func words(values ...string) *expression.List {
	return &expression.List{Values: values}
}

// matchesString builds a regular-expression test whose pattern is a string constant rather than
// the untyped constant Ref.Matches builds.
func matchesString(ref builder.Ref, pattern string) *expression.Call {
	return p.Compare(expression.OpRegex, ref, p.String(pattern))
}

func TestValidateFilter_Accepts(t *testing.T) {
	tests := []struct {
		name   string
		filter *expression.Call
	}{
		{
			name:   "unqualified attribute equality",
			filter: p.Attr("http.status_code").Eq("500"),
		},
		{
			name:   "phrase over an attribute",
			filter: p.Attr("gen_ai.prompt").Phrase("refund", "policy"),
		},
		{
			// The builder's text search takes bare words, so a list with a declared type is
			// built by hand.
			name: "fulltext over an event attribute with a declared string type",
			filter: &expression.Call{Op: expression.OpFulltext, Args: []expression.Expression{
				&expression.AttributeRef{Level: expression.LevelEvent, Key: "gen_ai.completion"},
				&expression.List{Values: []string{"refund"}, Type: expression.ValueTypeString},
			}},
		},
		{
			name:   "fulltext over words written in other scripts, with marks and digits",
			filter: p.Attr("a").Fulltext("naïve", "日本語", "b2", "Ṽ", strings.Repeat("a", 255)),
		},
		{
			name:   "text ordered against text",
			filter: p.Span().Name.Gt(p.String("m")),
		},
		{
			name:   "an attribute ordered against a number, since storage decides its type",
			filter: p.Attr("size").Gt(p.Int(500)),
		},
		{
			name:   "a timestamp field against an instant",
			filter: p.Span().StartTime.Lt(&expression.TimestampValue{Value: time.Unix(0, 0).UTC()}),
		},
		{
			name:   "level-qualified attribute",
			filter: p.Resource().Attr("k8s.pod.name").Eq(p.String("cart-0")),
		},
		{
			name:   "built-in field against a duration",
			filter: p.Span().Duration.Gt("2s"),
		},
		{
			name:   "the same attribute key at two levels",
			filter: p.Span().Attr("enduser.id").Eq(p.Resource().Attr("enduser.id")),
		},
		{
			name:   "two attributes ordered against each other",
			filter: p.Attr("queue.depth.after").Gt(p.Attr("queue.depth.before")),
		},
		{
			name:   "two built-in fields holding the same kind of value",
			filter: p.Span().StartTime.Lt(p.Span().EndTime),
		},
		{
			name:   "two constants, which asks nothing about the span but is still a comparison",
			filter: p.RefOf(p.Int(1)).Eq(p.Int(1)),
		},
		{
			name: "conjunction of two predicates",
			filter: p.And(
				p.Attr("a").Eq("1"),
				p.Attr("b").Eq("2"),
			),
		},
		{
			name: "nested disjunction under a negation",
			filter: p.Not(p.Or(
				p.Attr("a").Eq("1"),
				p.Attr("b").Eq("2"),
			)),
		},
		{
			name:   "set membership",
			filter: p.Attr("http.status_code").In(p.List(expression.ValueTypeInt, "500", "503")),
		},
		{
			name:   "existence of an attribute",
			filter: p.Attr("error").Exists(),
		},
		{
			name:   "a regular expression over a field",
			filter: p.Span().Name.Matches("GET .*"),
		},
		{
			name:   "a regular expression anchored at the start",
			filter: p.Span().Name.Matches("^GET"),
		},
		{
			name:   "a regular expression anchored at the end",
			filter: p.Span().Name.Matches(`\.json$`),
		},
		{
			name:   "a regular expression anchored at both ends",
			filter: p.Span().Name.Matches("^GET /api/[a-z]+$"),
		},
		{
			name:   "membership of a field holding one of a set of words in a list of strings",
			filter: p.Span().Kind.In(p.List(expression.ValueTypeString, "server", "client")),
		},
		{
			name:   "a comparison written with the constant on the left",
			filter: p.RefOf(&expression.AnyValue{Value: "2s"}).Gt(p.Span().Duration),
		},
		{
			name:   "a regular expression over a field holding one of a set of words",
			filter: matchesString(p.Span().Kind, "produ.*"),
		},
		{
			name:   "a regular expression folding case",
			filter: matchesString(p.Span().Name, "(?i)^get"),
		},
		{
			name:   "a regular expression with a typed pattern",
			filter: matchesString(p.Attr("http.route"), "/api/.*"),
		},
		{
			name:   "an ordered comparison of typed constants",
			filter: p.Attr("http.response.size").Gte(p.Int(500)),
		},
		{
			name: "correlated match over the event collection",
			filter: p.Some(p.Event(), p.And(
				p.Event().Name.Eq(p.String("exception")),
				p.Event().TimeSinceStart.Gt("50us"),
			)),
		},
		{
			name:   "a quantifier nested over the other collection",
			filter: p.Some(p.Event(), p.Some(p.Link(), p.Link().TraceID.Exists())),
		},
		{
			name: "two quantifiers over the same level side by side",
			filter: p.And(
				p.Some(p.Event(), p.Event().Name.Eq(p.String("exception"))),
				p.Some(p.Event(), p.Event().Name.Eq(p.String("retry"))),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter))
		})
	}
}

// TestValidateFilter_Rejects holds the trees the validator refuses. The builder fixes each
// operator's arity and operand kinds, so a case whose fault is the shape of the call itself is
// built by hand; a case whose fault is a term the builder takes as given, such as a level it does
// not know or a constant where a reference belongs, goes through RefOf.
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
			filter:      &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{p.Attr("a").Eq("1")}},
		},
		{
			name:        "conjunction over a bare reference",
			expectedErr: `operator "and" takes predicates as arguments, got an attribute reference`,
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				attr("a"),
				p.Attr("b").Eq("1"),
			}},
		},
		{
			name:        "invalid predicate nested in a conjunction",
			expectedErr: `unknown filter level "pod"`,
			filter: p.And(
				p.Attr("a").Eq("1"),
				p.RefOf(&expression.AttributeRef{Key: "b", Level: "pod"}).Eq("2"),
			),
		},
		{
			name:        "negation of two predicates",
			expectedErr: `operator "not" takes 1 argument(s), got 2`,
			filter: &expression.Call{Op: expression.OpNot, Args: []expression.Expression{
				p.Attr("a").Eq("1"),
				p.Attr("b").Eq("2"),
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
			filter:      eq(p.Int(1), p.String("1")),
		},
		{
			name:        "existence of a constant",
			expectedErr: `operator "exists" takes a reference, got an untyped constant`,
			filter:      p.RefOf(&expression.AnyValue{Value: "a"}).Exists(),
		},
		{
			name:        "existence of an attribute with no key",
			expectedErr: "attribute reference has no key",
			filter:      p.Span().Attr("").Exists(),
		},
		{
			name:        "existence of a field with no name",
			expectedErr: "field reference has no name",
			filter:      p.Span().Field("").Exists(),
		},
		{
			name:        "a field with no level",
			expectedErr: "field reference has no level, and a built-in field belongs to one",
			filter:      p.RefOf(&expression.FieldRef{Name: expression.SpanFieldDuration}).Eq("2s"),
		},
		{
			name:        "a field at an unknown level",
			expectedErr: `unknown filter level "pod"`,
			filter:      p.RefOf(&expression.FieldRef{Name: expression.SpanFieldDuration, Level: "pod"}).Eq("2s"),
		},
		{
			name:        "phrase over a built-in field",
			expectedErr: `operator "phrase" searches an attribute, not the built-in field "name" of the "span" level`,
			filter:      p.Span().Name.Phrase("a"),
		},
		{
			name:        "phrase over a collection",
			expectedErr: `a collection reference is only the first argument of "some"`,
			filter:      p.RefOf(&expression.NestedRef{Level: expression.LevelEvent}).Phrase("a"),
		},
		{
			name:        "fulltext over a constant",
			expectedErr: `operator "fulltext" takes a reference, got a string constant`,
			filter:      p.RefOf(p.String("a")).Fulltext("a"),
		},
		{
			name:        "fulltext over an attribute with no key",
			expectedErr: "attribute reference has no key",
			filter:      p.Attr("").Fulltext("a"),
		},
		{
			name:        "fulltext over a missing attribute reference",
			expectedErr: "filter has a missing reference",
			filter:      p.RefOf((*expression.AttributeRef)(nil)).Fulltext("a"),
		},
		{
			name:        "phrase of a missing list",
			expectedErr: `operator "phrase" takes a list of words as its second argument, got an empty term`,
			filter:      &expression.Call{Op: expression.OpPhrase, Args: []expression.Expression{attr("a"), (*expression.List)(nil)}},
		},
		{
			name:        "phrase with one argument",
			expectedErr: `operator "phrase" takes 2 argument(s), got 1`,
			filter:      &expression.Call{Op: expression.OpPhrase, Args: []expression.Expression{attr("a")}},
		},
		{
			name:        "phrase of a search string rather than a list",
			expectedErr: `operator "phrase" takes a list of words as its second argument, got a string constant`,
			filter:      &expression.Call{Op: expression.OpPhrase, Args: []expression.Expression{attr("a"), &expression.StringValue{Value: "refund policy"}}},
		},
		{
			name:        "phrase of an empty list",
			expectedErr: `operator "phrase" takes a list with at least one word`,
			filter:      p.Attr("a").Phrase(),
		},
		{
			name:        "fulltext of a list of integers",
			expectedErr: `operator "fulltext" takes a list of strings, not of "int"`,
			filter:      &expression.Call{Op: expression.OpFulltext, Args: []expression.Expression{attr("a"), &expression.List{Values: []string{"1"}, Type: expression.ValueTypeInt}}},
		},
		{
			name:        "fulltext of an empty word",
			expectedErr: `operator "fulltext": a word is empty`,
			filter:      p.Attr("a").Fulltext("refund", ""),
		},
		{
			name:        "phrase of a word holding a space, which the caller splits",
			expectedErr: `operator "phrase": a word holds letters, marks and digits only, got "refund policy"`,
			filter:      p.Attr("a").Phrase("refund policy"),
		},
		{
			name:        "phrase of a word holding punctuation",
			expectedErr: `operator "phrase": a word holds letters, marks and digits only, got "don't"`,
			filter:      p.Attr("a").Phrase("don't"),
		},
		{
			name:        "fulltext of a word longer than Elasticsearch keeps whole, measured in UTF-16 code units",
			expectedErr: `operator "fulltext": a word is at most 255 UTF-16 code units long, got one of 256`,
			filter:      p.Attr("a").Fulltext(strings.Repeat("a", 254) + "𝔸"),
		},
		{
			name:        "membership of a constant rather than a list",
			expectedErr: `operator "in" takes a list as its second argument, got an untyped constant`,
			filter:      &expression.Call{Op: expression.OpIn, Args: []expression.Expression{attr("a"), &expression.AnyValue{Value: "1"}}},
		},
		{
			name:        "membership in an empty list",
			expectedErr: `operator "in" takes a list with at least one element`,
			filter:      p.Attr("a").In(&expression.List{}),
		},
		{
			name:        "membership of a constant subject",
			expectedErr: `operator "in" takes a reference, got a string constant`,
			filter:      p.RefOf(p.String("a")).In("1"),
		},
		{
			name:        "list with an unknown type",
			expectedErr: `unknown filter value type "number"`,
			filter:      p.Attr("a").NotIn(p.List("number", "1")),
		},
		{
			name:        "list compared with equality",
			expectedErr: `operator "eq" compares a reference or a constant, got a list`,
			filter:      p.Attr("a").Eq(words("1")),
		},
		{
			name:        "a call result as an operand",
			expectedErr: `operator "eq" compares a reference or a constant, got a predicate`,
			filter:      p.RefOf(p.Attr("a").Exists()).Eq(p.Bool(true)),
		},
		{
			name:        "a call result as the subject of membership",
			expectedErr: `operator "in" takes a reference, got a predicate`,
			filter:      p.RefOf(p.Attr("a").Exists()).In(p.List(expression.ValueTypeBool, "true")),
		},
		{
			name:        "an ordered comparison of a word-valued field",
			expectedErr: `operator "gt" has no ordering for span.kind`,
			filter:      p.Span().Kind.Gt("server"),
		},
		{
			name:        "an ordered comparison of a boolean",
			expectedErr: `operator "gt" has no ordering for a boolean constant`,
			filter:      p.Attr("ok").Gt(p.Bool(true)),
		},
		{
			name:        "a duration field against a number",
			expectedErr: `operator "gt" compares span.duration against an integer constant, which hold different kinds of value`,
			filter:      p.Span().Duration.Gt(p.Int(42)),
		},
		{
			name:        "an argument with no term",
			expectedErr: `operator "eq" compares a reference or a constant, got an empty term`,
			filter:      &expression.Call{Op: expression.OpEq, Args: []expression.Expression{attr("a"), nil}},
		},
		{
			name:        "an ordered comparison against text",
			expectedErr: `operator "gt" compares span.duration against a string constant, which hold different kinds of value`,
			filter:      p.Span().Duration.Gt(p.String("2s")),
		},
		{
			name:        "an ordered comparison of constants of different kinds",
			expectedErr: `operator "gte" compares an integer constant against a boolean constant, which hold different kinds of value`,
			filter:      p.RefOf(p.Int(1)).Gte(p.Bool(true)),
		},
		{
			name:        "an ordered comparison against a boolean",
			expectedErr: `operator "lte" has no ordering for a boolean constant`,
			filter:      p.Attr("a").Lte(p.Bool(true)),
		},
		{
			name:        "two built-in fields holding different kinds of value",
			expectedErr: `operator "eq" compares span.duration against span.name, which hold different kinds of value`,
			filter:      p.Span().Duration.Eq(p.Span().Name),
		},
		{
			name:        "an instant ordered against a duration",
			expectedErr: `operator "gt" compares span.startTime against span.duration, which hold different kinds of value`,
			filter:      p.Span().StartTime.Gt(p.Span().Duration),
		},
		{
			name:        "an integer against a duration field",
			expectedErr: `operator "eq" compares span.duration against an integer constant, which hold different kinds of value`,
			filter:      p.Span().Duration.Eq(p.Int(2)),
		},
		{
			name:        "a timestamp against a duration field",
			expectedErr: `operator "gt" compares span.duration against a timestamp constant, which hold different kinds of value`,
			filter:      p.Span().Duration.Gt(&expression.TimestampValue{Value: time.Unix(0, 0).UTC()}),
		},
		{
			name:        "a duration against a text field",
			expectedErr: `operator "eq" compares span.name against a duration constant, which hold different kinds of value`,
			filter:      p.Span().Name.Eq(p.Duration(time.Second)),
		},
		{
			name:        "a number against a field holding one of a set of words",
			expectedErr: `operator "eq" compares span.kind against an integer constant, which hold different kinds of value`,
			filter:      p.Span().Kind.Eq(p.Int(1)),
		},
		{
			name:        "a boolean against a text field",
			expectedErr: `operator "eq" compares span.name against a boolean constant, which hold different kinds of value`,
			filter:      p.Span().Name.Eq(p.Bool(true)),
		},
		{
			name:        "a duration constant compared against an attribute",
			expectedErr: `operator "gt" compares a duration constant against an attribute, and the wire has no duration type`,
			filter:      p.Attr("latency").Gt(p.Duration(2 * time.Second)),
		},
		{
			name:        "a timestamp constant compared against an attribute",
			expectedErr: `operator "eq" compares a timestamp constant against an attribute, and the wire has no timestamp type`,
			filter:      p.Attr("deadline").Eq(&expression.TimestampValue{Value: time.Unix(0, 0).UTC()}),
		},
		{
			name:        "a regular expression that will not compile",
			expectedErr: `operator "regex" takes a pattern in RE2 syntax`,
			filter:      matchesString(p.Span().Name, "GET ("),
		},
		{
			name:        "a regular expression with a word boundary",
			expectedErr: `operator "regex" takes a pattern without word boundaries`,
			filter:      matchesString(p.Span().Name, `\bGET`),
		},
		{
			name:        "a regular expression with a lazy quantifier",
			expectedErr: `operator "regex" asks whether the value matches, so a quantifier cannot be lazy`,
			filter:      matchesString(p.Span().Name, "GET.*?/"),
		},
		{
			name:        "a regular expression anchored inside an alternation",
			expectedErr: `operator "regex" honors an anchor only as the first or the last element of the pattern`,
			filter:      matchesString(p.Span().Name, "^GET|^POST"),
		},
		{
			name:        "a regular expression anchored inside a group",
			expectedErr: `operator "regex" honors an anchor only as the first or the last element of the pattern`,
			filter:      matchesString(p.Span().Name, "(^GET)"),
		},
		{
			name:        "a regular expression anchored in the middle",
			expectedErr: `operator "regex" honors an anchor only as the first or the last element of the pattern`,
			filter:      matchesString(p.Span().Name, "GET^x"),
		},
		{
			name:        "a regular expression with multi-line anchors",
			expectedErr: `operator "regex" matches the value as a whole, so a pattern cannot use multi-line anchors`,
			filter:      matchesString(p.Span().Name, "(?m)^GET"),
		},
		{
			name:        "a regular expression over a duration field",
			expectedErr: `operator "regex" matches text, and span.duration holds a duration`,
			filter:      matchesString(p.Span().Duration, "2s"),
		},
		{
			name:        "a regular expression over a timestamp field",
			expectedErr: `operator "regex" matches text, and span.startTime holds a timestamp`,
			filter:      matchesString(p.Span().StartTime, "2026-.*"),
		},
		{
			name:        "a regular expression over a numeric pattern",
			expectedErr: `operator "regex" takes a constant string as its pattern, got an integer constant`,
			filter:      p.Compare(expression.OpRegex, p.Attr("a"), p.Int(1)),
		},
		{
			name:        "a regular expression matched against a reference",
			expectedErr: `operator "regex" takes a constant string as its pattern, got an attribute reference`,
			filter:      p.Compare(expression.OpRegex, p.Attr("a"), p.Attr("b")),
		},
		{
			name:        "a regular expression of one argument",
			expectedErr: `operator "regex" takes 2 argument(s), got 1`,
			filter:      &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{attr("a")}},
		},
		{
			name:        "a regular expression over a constant",
			expectedErr: `operator "regex" takes a reference, got a string constant`,
			filter:      matchesString(p.RefOf(p.String("a")), "b"),
		},
		{
			// Some takes a collection and a predicate, so a quantifier whose fault is in either
			// argument's kind is built by hand, here and in the cases below.
			name:        "quantifier over a constant",
			expectedErr: `operator "some" takes a collection reference as its first argument, got an untyped constant`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.AnyValue{Value: "event"},
				p.Event().Name.Exists(),
			}},
		},
		{
			name:        "quantifier over an attribute",
			expectedErr: `operator "some" takes a collection reference as its first argument, got an attribute reference`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.AttributeRef{Key: "a", Level: expression.LevelEvent},
				p.Attr("a").Exists(),
			}},
		},
		{
			name:        "quantifier over the span",
			expectedErr: `operator "some" quantifies over "event" or "link", got level "span"`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelSpan},
				p.Attr("a").Exists(),
			}},
		},
		{
			name:        "quantifier over a level-less collection",
			expectedErr: `operator "some" quantifies over "event" or "link", got level ""`,
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{},
				p.Attr("a").Exists(),
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
			filter:      p.Some(p.Link(), &expression.Call{Op: "matches", Args: []expression.Expression{attr("a")}}),
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
				p.Attr("a").Eq("1"),
				words("1"),
			}},
		},
		{
			name:        "membership with an invalid left operand",
			expectedErr: `unknown filter level "pod"`,
			filter:      p.RefOf(&expression.AttributeRef{Key: "a", Level: "pod"}).In("1"),
		},
		{
			name:        "invalid nested call as an operand",
			expectedErr: `operator "eq" compares a reference or a constant, got a predicate`,
			filter:      p.RefOf(&expression.Call{Op: "matches", Args: []expression.Expression{attr("a")}}).Eq("1"),
		},
		{
			name:        "invalid nested call as the subject of membership",
			expectedErr: `operator "in" takes a reference, got a predicate`,
			filter:      p.RefOf(&expression.Call{Op: "matches", Args: []expression.Expression{attr("a")}}).In("1"),
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
	collection := p.RefOf(&expression.NestedRef{Level: expression.LevelEvent})

	tests := map[string]*expression.Call{
		"compared against a constant":      collection.Eq("1"),
		"under exists":                     collection.Exists(),
		"as the subject of membership":     collection.In("1"),
		"as the predicate of a quantifier": p.Some(p.Link(), collection.Exists()),
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
	inner := func(collection builder.Collection) *expression.Call {
		return p.Some(collection, p.Event().Name.Exists())
	}
	directly := p.Some(p.Event(), inner(p.Event()))
	require.ErrorContains(t, ValidateFilter(directly),
		`operator "some" is already quantifying over "event", and this version does not define what a nested one would bind`)

	// However deep the inner one sits, and whichever level is doubled up.
	deeper := p.Some(p.Link(), p.And(
		p.Link().TraceID.Eq(p.String("abc")),
		p.Not(inner(p.Link())),
	))
	require.ErrorContains(t, ValidateFilter(deeper), `already quantifying over "link"`)
}

// TestValidateFilter_RejectsUnknownField pins that naming a field this API does not define is
// refused, and that the message says how to ask for an attribute of that name instead.
func TestValidateFilter_RejectsUnknownField(t *testing.T) {
	err := ValidateFilter(p.Span().Field("durtion").Eq("2s"))
	require.ErrorContains(t, err, `unknown built-in field "durtion" at the "span" level`)
	require.ErrorContains(t, err, "name an attribute to match a tag of that name instead")

	// The same name as an attribute is fine, because an attribute key is arbitrary.
	require.NoError(t, ValidateFilter(p.Span().Attr("durtion").Eq("2s")))

	// A field of the wrong level is refused too.
	require.ErrorContains(t,
		ValidateFilter(p.Resource().Field(expression.SpanFieldDuration).Eq("2s")),
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
			ref := p.RefOf(&expression.AttributeRef{Level: level, Key: "a"})
			require.NoError(t, ValidateFilter(ref.Eq("1")))
		})
	}
	require.NoError(t, ValidateFilter(p.Attr("a").Eq("1")),
		"an empty level is the unqualified attribute and is always allowed")
}

// TestValidateFilter_AcceptsEveryValueType pins that a list may declare any of the defined types,
// and that it may also declare none. Declaring none means one of two things, which the last two
// subjects distinguish: the built-in field the list is compared against supplies the type, or the
// list sits beside an attribute, where nothing supplies one and the elements are under no type
// constraint.
func TestValidateFilter_AcceptsEveryValueType(t *testing.T) {
	for _, vt := range expression.ValueTypes() {
		t.Run(string(vt), func(t *testing.T) {
			require.NoError(t, ValidateFilter(p.Attr("a").In(p.List(vt, "1"))))
		})
	}

	t.Run("no type, against a built-in field", func(t *testing.T) {
		require.NoError(t, ValidateFilter(p.Span().Duration.In("2s")))
	})

	// An attribute is under no type constraint, so a list beside one needs no declared type
	// either — the same concession `eq` against an attribute has always made for a scalar.
	t.Run("no type, against an attribute", func(t *testing.T) {
		require.NoError(t, ValidateFilter(p.Attr("a").In("1")))
	})
}

// TestValidateFilter_AcceptsEveryConstant pins that every constant node is comparable against
// something, since each one is what a wire hint or a resolution can produce. A duration and an
// instant are comparable only against a built-in field that declares that type (§5.4), which is why
// the reference this compares against depends on the constant.
func TestValidateFilter_AcceptsEveryConstant(t *testing.T) {
	for _, test := range allTerms {
		if !isConstant(test.term) {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			reference := p.Attr("a")
			switch test.term.(type) {
			case *expression.DurationValue:
				reference = p.Span().Duration
			case *expression.TimestampValue:
				reference = p.Span().StartTime
			default:
				// Every other constant is comparable against an attribute, which declares
				// no type of its own.
			}
			require.NoError(t, ValidateFilter(reference.Eq(test.term)))
		})
	}
}

// TestValidateFilter_AcceptsEveryField pins that the field enumeration and the validator agree:
// every field Fields() offers is one a query may actually name at that level.
func TestValidateFilter_AcceptsEveryField(t *testing.T) {
	for _, f := range expression.Fields() {
		t.Run(string(f.Level)+"."+f.Name, func(t *testing.T) {
			ref := p.RefOf(&expression.FieldRef{Level: f.Level, Name: f.Name})
			require.NoError(t, ValidateFilter(ref.Eq("x")))
		})
	}
}

// TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth pins that validation recurses down every
// path a node can hide in, not just the root. Each case buries the same bad reference — an
// unknown level — somewhere different.
func TestValidateFilter_CatchesAnInvalidNodeAtAnyDepth(t *testing.T) {
	bad := p.RefOf(&expression.AttributeRef{Level: "nonesuch", Key: "a"})
	good := p.Attr("ok").Eq("1")

	tests := map[string]*expression.Call{
		"under and":               p.And(good, bad.Eq("1")),
		"under or":                p.Or(good, bad.Eq("1")),
		"under not":               p.Not(bad.Eq("1")),
		"as a comparison operand": bad.Eq("1"),
		"as the right operand":    p.Attr("ok").Eq(bad),
		"as the subject of in":    bad.In("1"),
		"as the subject of regex": matchesString(bad, "1"),
		"under exists":            bad.Exists(),
		"inside a some predicate": p.Some(p.Event(), bad.Eq("1")),
		// And flattens a nested conjunction, so the one that stays nested is built by hand.
		"two conjunctions deep": {Op: expression.OpAnd, Args: []expression.Expression{
			good,
			&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{good, bad.Eq("1")}},
		}},
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
// returns. A validator that panicked would take the process down on a hostile request. The trees
// are built by hand because the builder cannot produce them.
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
	filter := p.Attr("a").Eq("1")
	for range depth - 1 {
		filter = p.Not(filter)
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
	quantified := p.Some(p.Event(), nestedTo(expression.MaxNestingDepth))
	require.ErrorIs(t, ValidateFilter(quantified), expression.ErrTooDeeplyNested)
}

// TestValidateFilter_RefusesAFilterThatContainsItself is the reason the bound exists rather than a
// cycle check: the AST is built from pointers, so a caller can hand a call itself as one of its own
// arguments, and the walk would follow it until the stack ran out. Being too deep is the answer.
// The cycles are built by hand because the builder's arguments are evaluated before the call.
func TestValidateFilter_RefusesAFilterThatContainsItself(t *testing.T) {
	cycle := &expression.Call{Op: expression.OpNot}
	cycle.Args = []expression.Expression{cycle}
	require.ErrorIs(t, ValidateFilter(cycle), expression.ErrTooDeeplyNested)

	outer := &expression.Call{Op: expression.OpAnd}
	inner := &expression.Call{Op: expression.OpNot, Args: []expression.Expression{outer}}
	outer.Args = []expression.Expression{inner, p.Attr("a").Eq("1")}
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
			return p.Attr("a").Eq(c)
		},
		"compared against a built-in field": func(c expression.Expression) *expression.Call {
			return p.Span().Duration.Gt(c)
		},
		"written on the left of a comparison": func(c expression.Expression) *expression.Call {
			return p.RefOf(c).Eq(p.Span().Name)
		},
		"as a regular expression": func(c expression.Expression) *expression.Call {
			return p.Compare(expression.OpRegex, p.Span().Name, c)
		},
		"as a list element's neighbour": func(c expression.Expression) *expression.Call {
			return p.And(
				p.Attr("a").In(p.List(expression.ValueTypeInt, "1")),
				p.Attr("b").Eq(c),
			)
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

// allTerms is every term type the AST defines, as the pointer a tree is built from, paired with
// the name an error message gives it. The tests below walk it, so a term type that jaeger-idl adds
// without a case in termName or domainOf fails here rather than being reported to a caller as an
// empty term or silently compared as a value of unknown type.
var allTerms = []struct {
	term expression.Expression
	name string
}{
	{&expression.AttributeRef{}, "an attribute reference"},
	{&expression.FieldRef{}, "a field reference"},
	{&expression.NestedRef{}, "a collection reference"},
	{&expression.AnyValue{}, "an untyped constant"},
	{&expression.StringValue{}, "a string constant"},
	{&expression.IntValue{}, "an integer constant"},
	{&expression.DoubleValue{}, "a floating-point constant"},
	{&expression.BoolValue{}, "a boolean constant"},
	{&expression.DurationValue{}, "a duration constant"},
	{&expression.TimestampValue{}, "a timestamp constant"},
	{&expression.List{}, "a list"},
	{&expression.Call{}, "a predicate"},
}

// TestTermNames pins the name a filter's rejection gives each term, which is the whole of what a
// caller learns about a term the validator would not take. A term type with no case here is
// reported as an unknown term, so this is what keeps the message honest as jaeger-idl grows terms.
func TestTermNames(t *testing.T) {
	for _, test := range allTerms {
		assert.Equal(t, test.name, termName(test.term))
	}
	assert.Equal(t, "an empty term", termName(nil))
}

// TestConstantKinds pins the kind of value each constant holds, and which ones can serve as a
// regular expression. An untyped constant holds nothing known, because it is what a value with no
// wire hint arrives as and no operator can settle its type from its text alone.
func TestConstantKinds(t *testing.T) {
	domains := map[string]domain{}
	patterns := map[string]bool{}
	for _, test := range allTerms {
		if !isConstant(test.term) {
			continue
		}
		domains[test.name] = domainOf(test.term)
		if _, ok := patternText(test.term); ok {
			patterns[test.name] = true
		}
	}
	assert.Equal(t, map[string]domain{
		"an untyped constant":       domainUnknown,
		"a string constant":         domainText,
		"an integer constant":       domainNumber,
		"a floating-point constant": domainNumber,
		"a boolean constant":        domainBool,
		"a duration constant":       domainDuration,
		"a timestamp constant":      domainTimestamp,
	}, domains)
	assert.Equal(t, map[string]bool{
		"an untyped constant": true,
		"a string constant":   true,
	}, patterns)

	assert.False(t, isConstant(&expression.List{}), "a list is only ever a membership operand")
	assert.False(t, isConstant(nil))
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
	err := ValidateFilter(p.Attr("a").Eq(term))
	require.ErrorContains(t, err, "an unknown term")
}
