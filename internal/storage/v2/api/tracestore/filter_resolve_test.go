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

func spanField(name string) *expression.FieldRef {
	return &expression.FieldRef{Name: name, Level: expression.LevelSpan}
}

// TestResolveConstants pins what each field type reads its constant as, and what resolution
// leaves alone: a constant that already has a type, a constant compared against an attribute,
// and a regular expression's pattern.
func TestResolveConstants(t *testing.T) {
	timestamp, err := time.Parse(time.RFC3339Nano, "2026-08-16T18:56:20.123456789Z")
	require.NoError(t, err)

	tests := []struct {
		name     string
		filter   *expression.Call
		expected *expression.Call
	}{
		{
			name:     "a duration field",
			filter:   &expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), &expression.AnyValue{Value: "2s"}}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), &expression.DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "a timestamp field",
			filter:   &expression.Call{Op: expression.OpGte, Args: []expression.Expression{spanField(expression.SpanFieldStartTime), &expression.AnyValue{Value: "2026-08-16T18:56:20.123456789Z"}}},
			expected: &expression.Call{Op: expression.OpGte, Args: []expression.Expression{spanField(expression.SpanFieldStartTime), &expression.TimestampValue{Value: timestamp}}},
		},
		{
			name:     "a string field",
			filter:   eq(spanField(expression.SpanFieldName), &expression.AnyValue{Value: "GET /api"}),
			expected: eq(spanField(expression.SpanFieldName), &expression.StringValue{Value: "GET /api"}),
		},
		{
			name:     "an event field, which measures from its span's start",
			filter:   &expression.Call{Op: expression.OpLt, Args: []expression.Expression{&expression.FieldRef{Name: expression.EventFieldTimeSinceStart, Level: expression.LevelEvent}, &expression.AnyValue{Value: "50us"}}},
			expected: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{&expression.FieldRef{Name: expression.EventFieldTimeSinceStart, Level: expression.LevelEvent}, &expression.DurationValue{Value: 50 * time.Microsecond}}},
		},
		{
			name:     "an attribute, which declares nothing",
			filter:   &expression.Call{Op: expression.OpGt, Args: []expression.Expression{attr("http.response.size"), &expression.AnyValue{Value: "500"}}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{attr("http.response.size"), &expression.AnyValue{Value: "500"}}},
		},
		{
			name:     "a constant that already has a type",
			filter:   eq(spanField(expression.SpanFieldName), &expression.StringValue{Value: "GET /api"}),
			expected: eq(spanField(expression.SpanFieldName), &expression.StringValue{Value: "GET /api"}),
		},
		{
			name:     "a pattern, which stays a pattern rather than becoming the field's type",
			filter:   &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{spanField(expression.SpanFieldName), &expression.AnyValue{Value: ".*"}}},
			expected: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{spanField(expression.SpanFieldName), &expression.AnyValue{Value: ".*"}}},
		},
		{
			name: "a membership list, which carries its elements as text",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				spanField(expression.SpanFieldDuration), &expression.List{Values: []string{"2s", "3s"}},
			}},
			expected: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				spanField(expression.SpanFieldDuration), &expression.List{Values: []string{"2s", "3s"}},
			}},
		},
		{
			name:     "a comparison against an attribute, whose type only storage knows",
			filter:   eq(attr("http.method"), &expression.AnyValue{Value: "GET"}),
			expected: eq(attr("http.method"), &expression.AnyValue{Value: "GET"}),
		},
		{
			name: "a predicate buried under a quantifier and a negation",
			filter: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpNot, Args: []expression.Expression{
					&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
						&expression.FieldRef{Name: expression.EventFieldTimeSinceStart, Level: expression.LevelEvent},
						&expression.AnyValue{Value: "1ms"},
					}},
				}},
			}},
			expected: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.NestedRef{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpNot, Args: []expression.Expression{
					&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
						&expression.FieldRef{Name: expression.EventFieldTimeSinceStart, Level: expression.LevelEvent},
						&expression.DurationValue{Value: time.Millisecond},
					}},
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter), "the fixture is a filter validation accepts")
			resolved, err := ResolveFilterConstants(test.filter)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}

// TestResolveConstants_LeavesAnUndefinedFieldAlone pins that resolution has nothing to say
// about a field this API does not define: there is no type to read the constant as, and
// ValidateFilter is what refuses the reference.
func TestResolveConstants_LeavesAnUndefinedFieldAlone(t *testing.T) {
	filter := eq(spanField("durtion"), &expression.AnyValue{Value: "2s"})
	require.Error(t, ValidateFilter(filter))

	resolved, err := ResolveFilterConstants(filter)
	require.NoError(t, err)
	assert.Equal(t, filter, resolved)
}

// TestResolveConstants_RefusesAConstantThatWillNotParse pins the point of resolving against the
// field: the query boundary answers a malformed value, rather than each backend interpreting it
// its own way.
func TestResolveConstants_RefusesAConstantThatWillNotParse(t *testing.T) {
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name:        "a word where a duration belongs",
			filter:      &expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), &expression.AnyValue{Value: "banana"}}},
			expectedErr: `cannot compare span.duration against "banana": time: invalid duration "banana"`,
		},
		{
			name:        "a bare number, since a duration carries its unit",
			filter:      &expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), &expression.AnyValue{Value: "500"}}},
			expectedErr: `cannot compare span.duration against "500"`,
		},
		{
			name:        "a timestamp that is not RFC 3339",
			filter:      &expression.Call{Op: expression.OpLt, Args: []expression.Expression{spanField(expression.SpanFieldEndTime), &expression.AnyValue{Value: "yesterday"}}},
			expectedErr: `cannot compare span.endTime against "yesterday"`,
		},
		{
			name: "buried in a conjunction",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eq(attr("a"), &expression.AnyValue{Value: "1"}),
				&expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), &expression.AnyValue{Value: "banana"}}},
			}},
			expectedErr: `cannot compare span.duration against "banana"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := ResolveFilterConstants(test.filter)
			require.ErrorContains(t, err, test.expectedErr)
			assert.Nil(t, resolved)
		})
	}
}

// TestResolveConstants_LeavesItsInputAlone pins that resolution rewrites nodes into a new tree
// rather than annotating the one it was given, which is what keeps a query interceptor's later
// edit from leaving anything stale behind.
// TestResolveConstants_ChecksMembershipElements pins that a value refused under a comparison
// is refused under membership too, and that a declared element type does not exempt the list
// from either half of that: the type has to be one the field could hold, and the elements have
// to be readable as it. The list is not rewritten, so what this asserts is the refusal.
func TestResolveConstants_ChecksMembershipElements(t *testing.T) {
	duration := &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}

	_, err := ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		duration, &expression.List{Values: []string{"2s", "banana"}},
	}})
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpNotIn, Args: []expression.Expression{
		duration, &expression.List{Values: []string{"2s", "3m"}},
	}})
	require.NoError(t, err)

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		duration, &expression.List{Values: []string{"banana"}, Type: expression.ValueTypeString},
	}})
	require.ErrorContains(t, err, "cannot compare span.duration against a list of string")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		duration, &expression.List{Values: []string{"12"}, Type: expression.ValueTypeInt},
	}})
	require.ErrorContains(t, err, "cannot compare span.duration against a list of int")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		duration, &expression.List{Values: []string{"true"}, Type: expression.ValueTypeBool},
	}})
	require.ErrorContains(t, err, "cannot compare span.duration against a list of bool")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan},
		&expression.List{Values: []string{"GET /a", "GET /b"}, Type: expression.ValueTypeString},
	}})
	require.NoError(t, err, "a declared type the field can hold, with elements that read as it")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.AttributeRef{Key: "size"}, &expression.List{Values: []string{"banana"}},
	}})
	require.NoError(t, err, "an attribute's values are storage's to read")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan},
		&expression.List{Values: []string{"server", "client"}, Type: expression.ValueTypeString},
	}})
	require.NoError(t, err, "a word-valued field holds text, so a list of strings suits it")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldStatus, Level: expression.LevelSpan},
		&expression.List{Values: []string{"banana"}, Type: expression.ValueTypeString},
	}})
	require.ErrorContains(t, err, "not one of unset, ok, error")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan},
		&expression.List{Values: []string{"true"}, Type: expression.ValueTypeBool},
	}})
	require.ErrorContains(t, err, "cannot compare span.name against a list of bool")

	// A declared element type is authoritative wherever it appears, so the elements are read as
	// it whether or not there is a field opposite to compare it with.
	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.AttributeRef{Key: "size"}, &expression.List{Values: []string{"banana"}, Type: expression.ValueTypeInt},
	}})
	require.ErrorContains(t, err, `element "banana" of a list of int`)

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.AttributeRef{Key: "size"}, &expression.List{Values: []string{"12", "banana"}, Type: expression.ValueTypeDouble},
	}})
	require.ErrorContains(t, err, `element "banana" of a list of double`)

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.AttributeRef{Key: "cache.hit"}, &expression.List{Values: []string{"yes"}, Type: expression.ValueTypeBool},
	}})
	require.ErrorContains(t, err, `element "yes" of a list of bool`)

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.AttributeRef{Key: "size"}, &expression.List{Values: []string{"12"}, Type: expression.ValueTypeInt},
	}})
	require.NoError(t, err, "elements that read as their declared type")

	// ValidateFilter refuses both of these, so resolution only has to not choke on them.
	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		duration, &expression.AnyValue{Value: "2s"},
	}})
	require.NoError(t, err, "membership without a list is ValidateFilter's refusal")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		&expression.FieldRef{Name: "nosuchfield", Level: expression.LevelSpan}, &expression.List{Values: []string{"banana"}},
	}})
	require.NoError(t, err, "an undefined field has no type to read against")
}

// TestResolveConstants_ChecksEnumSpellings pins the two fields that hold one of a closed set of
// words. A word outside the set can never match any span, so it is answered here with the set,
// rather than passed to a backend that would return nothing and say why.
func TestResolveConstants_ChecksEnumSpellings(t *testing.T) {
	kind := &expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan}
	status := &expression.FieldRef{Name: expression.SpanFieldStatus, Level: expression.LevelSpan}

	got, err := ResolveFilterConstants(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{kind, &expression.AnyValue{Value: "server"}}})
	require.NoError(t, err)
	assert.Equal(t, &expression.StringValue{Value: "server"}, got.Args[1])

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{kind, &expression.AnyValue{Value: "SPAN_KIND_SERVER"}}})
	require.ErrorContains(t, err, `cannot compare span.kind against "SPAN_KIND_SERVER"`)
	require.ErrorContains(t, err, "not one of unspecified, internal, server, client, producer, consumer")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{status, &expression.AnyValue{Value: "Error"}}})
	require.ErrorContains(t, err, "not one of unset, ok, error")

	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpIn, Args: []expression.Expression{
		status, &expression.List{Values: []string{"ok", "banana"}},
	}})
	require.ErrorContains(t, err, `cannot compare span.status against "banana"`)

	// An ID stays a string: one nobody recorded reads the same as one being looked for.
	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldTraceID, Level: expression.LevelSpan}, &expression.AnyValue{Value: "not-hex"},
	}})
	require.NoError(t, err)

	// Declaring the constant a string does not get it past the vocabulary. Validation has nothing
	// to say about it: span.status holds text and so does the constant.
	_, err = ResolveFilterConstants(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{status, &expression.StringValue{Value: "banana"}}})
	require.ErrorContains(t, err, `cannot compare span.status against "banana"`)
	require.ErrorContains(t, err, "not one of unset, ok, error")

	got, err = ResolveFilterConstants(&expression.Call{Op: expression.OpEq, Args: []expression.Expression{kind, &expression.StringValue{Value: "client"}}})
	require.NoError(t, err)
	assert.Equal(t, &expression.StringValue{Value: "client"}, got.Args[1])
}

// TestResolveConstants_AcceptsCompatibleTypedConstants is the other half: a constant whose
// declared type the field holds needs no rewriting and is passed through as it stands.
func TestResolveConstants_AcceptsCompatibleTypedConstants(t *testing.T) {
	tests := []struct {
		name     string
		filter   *expression.Call
		expected expression.Expression
	}{
		{
			name: "a duration against a duration field",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				spanField(expression.SpanFieldDuration), &expression.DurationValue{Value: 2 * time.Second},
			}},
			expected: &expression.DurationValue{Value: 2 * time.Second},
		},
		{
			name: "a string against a text field",
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				spanField(expression.SpanFieldName), &expression.StringValue{Value: "GET /"},
			}},
			expected: &expression.StringValue{Value: "GET /"},
		},
		{
			name: "a boolean against a field that holds one, which no built-in field does",
			filter: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.AttributeRef{Key: "cache.hit"}, &expression.BoolValue{Value: true},
			}},
			expected: &expression.BoolValue{Value: true},
		},
		{
			name: "an integer against an attribute, whose type only storage knows",
			filter: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.status_code"}, &expression.IntValue{Value: 500},
			}},
			expected: &expression.IntValue{Value: 500},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := ResolveFilterConstants(test.filter)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved.Args[1])
		})
	}
}

// TestResolveConstants_PutsTheReferenceFirst pins the orientation every consumer downstream
// relies on. A caller may write the constant on the left, so resolution swaps the operands and
// inverts an ordered operator, which leaves the query asking the same thing.
func TestResolveConstants_PutsTheReferenceFirst(t *testing.T) {
	duration := spanField(expression.SpanFieldDuration)
	name := spanField(expression.SpanFieldName)

	tests := []struct {
		name     string
		filter   *expression.Call
		expected *expression.Call
	}{
		{
			name:     "greater than becomes less than",
			filter:   &expression.Call{Op: expression.OpGt, Args: []expression.Expression{&expression.AnyValue{Value: "2s"}, duration}},
			expected: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{duration, &expression.DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "at least becomes at most",
			filter:   &expression.Call{Op: expression.OpGte, Args: []expression.Expression{&expression.AnyValue{Value: "2s"}, duration}},
			expected: &expression.Call{Op: expression.OpLte, Args: []expression.Expression{duration, &expression.DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "less than becomes greater than",
			filter:   &expression.Call{Op: expression.OpLt, Args: []expression.Expression{&expression.AnyValue{Value: "2s"}, duration}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{duration, &expression.DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "at most becomes at least",
			filter:   &expression.Call{Op: expression.OpLte, Args: []expression.Expression{&expression.AnyValue{Value: "2s"}, duration}},
			expected: &expression.Call{Op: expression.OpGte, Args: []expression.Expression{duration, &expression.DurationValue{Value: 2 * time.Second}}},
		},
		{
			name:     "equality keeps its operator",
			filter:   &expression.Call{Op: expression.OpEq, Args: []expression.Expression{&expression.AnyValue{Value: "GET /"}, name}},
			expected: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{name, &expression.StringValue{Value: "GET /"}}},
		},
		{
			name:     "inequality keeps its operator",
			filter:   &expression.Call{Op: expression.OpNe, Args: []expression.Expression{&expression.AnyValue{Value: "GET /"}, name}},
			expected: &expression.Call{Op: expression.OpNe, Args: []expression.Expression{name, &expression.StringValue{Value: "GET /"}}},
		},
		{
			name:     "a comparison already the right way round is left alone",
			filter:   &expression.Call{Op: expression.OpGt, Args: []expression.Expression{duration, &expression.AnyValue{Value: "2s"}}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{duration, &expression.DurationValue{Value: 2 * time.Second}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter), "the fixture is a filter validation accepts")
			resolved, err := ResolveFilterConstants(test.filter)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}

// TestResolveConstants_LeavesTwoReferencesAlone pins that resolution answers for a tree
// validation would have refused. There is no constant to read between two references, and
// refusing the comparison is ValidateFilter's job rather than this stage's.
// TestResolveConstants_ReadsEveryListElement walks the list forms a filter can carry, since the
// element type comes from two different places and an element that cannot be read as it is the
// one thing membership refuses.
func TestResolveConstants_ReadsEveryListElement(t *testing.T) {
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name: "a declared type its elements read as",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				attr("size"), &expression.List{Values: []string{"1", "2"}, Type: expression.ValueTypeInt},
			}},
		},
		{
			name: "durations, whose type the field supplies",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				spanField(expression.SpanFieldDuration), &expression.List{Values: []string{"2s", "50us"}},
			}},
		},
		{
			name: "instants, whose type the field supplies",
			filter: &expression.Call{Op: expression.OpNotIn, Args: []expression.Expression{
				spanField(expression.SpanFieldStartTime), &expression.List{Values: []string{"2026-08-18T00:00:00Z"}},
			}},
		},
		{
			name: "words a closed set holds",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				spanField(expression.SpanFieldKind), &expression.List{Values: []string{"server", "client"}, Type: expression.ValueTypeString},
			}},
		},
		{
			name: "elements of mixed types under one declared type",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				attr("size"), &expression.List{Values: []string{"1", "true"}, Type: expression.ValueTypeInt},
			}},
			expectedErr: `element "true" of a list of int`,
		},
		{
			name: "a duration among instants",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				spanField(expression.SpanFieldStartTime), &expression.List{Values: []string{"2026-08-18T00:00:00Z", "2s"}},
			}},
			expectedErr: `cannot compare span.startTime against "2s"`,
		},
		{
			name: "a word outside the set the field holds",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				spanField(expression.SpanFieldKind), &expression.List{Values: []string{"server", "banana"}, Type: expression.ValueTypeString},
			}},
			expectedErr: "not one of unspecified, internal, server, client, producer, consumer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateFilter(test.filter), "the fixture is a filter validation accepts")
			resolved, err := ResolveFilterConstants(test.filter)
			if test.expectedErr != "" {
				require.ErrorContains(t, err, test.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.filter.Args[1], resolved.Args[1], "a list is carried over as it stands")
		})
	}
}

func TestResolveConstants_LeavesTwoReferencesAlone(t *testing.T) {
	filter := &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
		spanField(expression.SpanFieldStartTime), spanField(expression.SpanFieldEndTime),
	}}
	resolved, err := ResolveFilterConstants(filter)
	require.NoError(t, err)
	assert.Equal(t, filter, resolved)
}

func TestResolveConstants_LeavesItsInputAlone(t *testing.T) {
	constant := &expression.AnyValue{Value: "2s"}
	filter := &expression.Call{Op: expression.OpGt, Args: []expression.Expression{spanField(expression.SpanFieldDuration), constant}}

	resolved, err := ResolveFilterConstants(filter)
	require.NoError(t, err)

	assert.Equal(t, &expression.DurationValue{Value: 2 * time.Second}, resolved.Args[1])
	assert.Same(t, constant, filter.Args[1], "the original constant is still the one it was")
	assert.NotSame(t, filter, resolved, "the tree is rebuilt rather than edited")
}

// TestResolveConstants_NeverPanics pins that resolution answers for any tree, the same way
// validation does, since a decoder can produce trees a caller could not build.
// TestResolveConstants_BoundsNesting pins that resolution bounds its own recursion. It is
// documented to answer for any tree, including one validation never saw, so a filter that contains
// itself has to stop here too rather than run the stack out.
// TestReadElement covers the one reading operation a consumer needs for a list: the type comes from
// the list where it declares one, and from the field opposite it where it does not. On a finalized
// filter it cannot fail, since finalizing refused every element that would not read.
func TestReadElement(t *testing.T) {
	tests := []struct {
		name      string
		list      *expression.List
		fieldType expression.FieldType
		element   string
		expected  expression.Expression
		wantErr   string
	}{
		{
			name:     "a declared integer",
			list:     &expression.List{Values: []string{"500"}, Type: expression.ValueTypeInt},
			element:  "500",
			expected: &expression.IntValue{Value: 500},
		},
		{
			name:     "a declared double",
			list:     &expression.List{Values: []string{"1.50"}, Type: expression.ValueTypeDouble},
			element:  "1.50",
			expected: &expression.DoubleValue{Value: 1.5},
		},
		{
			name:     "a declared boolean",
			list:     &expression.List{Values: []string{"true"}, Type: expression.ValueTypeBool},
			element:  "true",
			expected: &expression.BoolValue{Value: true},
		},
		{
			name:     "a declared string",
			list:     &expression.List{Values: []string{"/cart"}, Type: expression.ValueTypeString},
			element:  "/cart",
			expected: &expression.StringValue{Value: "/cart"},
		},
		{
			name:      "a duration, whose type the field supplies",
			list:      &expression.List{Values: []string{"2s"}},
			fieldType: expression.FieldTypeDuration,
			element:   "2s",
			expected:  &expression.DurationValue{Value: 2 * time.Second},
		},
		{
			name:      "a word the field's closed set holds",
			list:      &expression.List{Values: []string{"server"}},
			fieldType: expression.FieldTypeSpanKind,
			element:   "server",
			expected:  &expression.StringValue{Value: "server"},
		},
		{
			name:      "a word outside that set",
			list:      &expression.List{Values: []string{"banana"}},
			fieldType: expression.FieldTypeSpanKind,
			element:   "banana",
			wantErr:   "not one of unspecified, internal, server, client, producer, consumer",
		},
		{
			name:    "an element that is not the type the list declares",
			list:    &expression.List{Values: []string{"banana"}, Type: expression.ValueTypeInt},
			element: "banana",
			wantErr: `element "banana" of a list of int`,
		},
		{
			// Beside an attribute nothing declares a type, and the element is under no type
			// constraint rather than unreadable.
			name:     "no type declared and no field to supply one",
			list:     &expression.List{Values: []string{"GET"}},
			element:  "GET",
			expected: &expression.AnyValue{Value: "GET"},
		},
		{
			name:    "no list at all",
			element: "500",
			wantErr: "list is empty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := ReadFilterElement(test.list, test.fieldType, test.element)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, value)
		})
	}
}

// TestReadConstant covers the same reading for a value the wire carried as text against a built-in
// field, which is what finalizing does and what a consumer holding an unfinalized tree needs.
func TestReadConstant(t *testing.T) {
	value, err := ReadFilterConstant(expression.FieldTypeDuration, "50us")
	require.NoError(t, err)
	assert.Equal(t, &expression.DurationValue{Value: 50 * time.Microsecond}, value)

	_, err = ReadFilterConstant(expression.FieldTypeTimestamp, "yesterday")
	require.Error(t, err)
}

func TestResolveConstants_BoundsNesting(t *testing.T) {
	resolved, err := ResolveFilterConstants(nestedTo(expression.MaxNestingDepth))
	require.NoError(t, err)
	assert.NotNil(t, resolved)

	_, err = ResolveFilterConstants(nestedTo(expression.MaxNestingDepth + 1))
	require.ErrorIs(t, err, expression.ErrTooDeeplyNested)

	cycle := &expression.Call{Op: expression.OpNot}
	cycle.Args = []expression.Expression{cycle}
	_, err = ResolveFilterConstants(cycle)
	require.ErrorIs(t, err, expression.ErrTooDeeplyNested)
}

func TestResolveConstants_NeverPanics(t *testing.T) {
	trees := map[string]*expression.Call{
		"no operator":                {},
		"no arguments":               {Op: expression.OpEq},
		"a nil argument":             {Op: expression.OpEq, Args: []expression.Expression{nil, nil}},
		"a nil nested call":          {Op: expression.OpAnd, Args: []expression.Expression{(*expression.Call)(nil), (*expression.Call)(nil)}},
		"a nil field reference":      {Op: expression.OpEq, Args: []expression.Expression{(*expression.FieldRef)(nil), &expression.AnyValue{Value: "1"}}},
		"one argument too few":       {Op: expression.OpEq, Args: []expression.Expression{spanField(expression.SpanFieldDuration)}},
		"a field against two things": {Op: expression.OpEq, Args: []expression.Expression{spanField(expression.SpanFieldDuration), &expression.AnyValue{Value: "2s"}, &expression.AnyValue{Value: "3s"}}},
	}
	for name, filter := range trees {
		t.Run(name, func(t *testing.T) {
			assert.NotPanics(t, func() { _, _ = ResolveFilterConstants(filter) })
		})
	}

	resolved, err := ResolveFilterConstants(nil)
	require.ErrorContains(t, err, "filter is empty")
	assert.Nil(t, resolved)
}

// TestReadConstant_RefusesAnUndeclaredType pins the answer when a field type has no rule for
// reading its constants, which is the state a type added to the vocabulary alone would leave.
func TestReadConstant_RefusesAnUndeclaredType(t *testing.T) {
	value, err := readConstant("nonesuch", "2s")
	require.ErrorContains(t, err, `no rule for reading a constant as "nonesuch"`)
	assert.Nil(t, value)
}

// TestFieldTypes_AreAllReadable pins that every type a built-in field declares has a rule for
// reading a constant as it. Without this, jaeger-idl adding a field of a new type would refuse
// every constant compared against that field, and only an end-to-end query would show it.
//
// The types come from the fields themselves, since a type no field declares is one no query can
// reach. TestReadConstant_RefusesAnUndeclaredType covers the other side, where a type has no rule.
func TestFieldTypes_AreAllReadable(t *testing.T) {
	require.NotEmpty(t, expression.Fields())
	seen := map[expression.FieldType]bool{}
	for _, f := range expression.Fields() {
		if seen[f.Type] {
			continue
		}
		seen[f.Type] = true
		t.Run(string(f.Type), func(t *testing.T) {
			value, err := readConstant(f.Type, textFor(f.Type))
			require.NoError(t, err)
			assert.NotNil(t, value)
		})
	}
}

// textFor is a value the given field type can be read from, so that walking the field types does
// not turn into a test of each type's parser.
func textFor(t expression.FieldType) string {
	switch t {
	case expression.FieldTypeDuration:
		return "2s"
	case expression.FieldTypeTimestamp:
		return "2026-08-16T18:56:20.123456789Z"
	case expression.FieldTypeSpanKind:
		return expression.SpanKinds()[0]
	case expression.FieldTypeSpanStatus:
		return expression.SpanStatuses()[0]
	default:
		return "anything"
	}
}
