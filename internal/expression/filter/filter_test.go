// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// TestLowering pins what each chain lowers to. The builder promises no more than the tree it
// writes, so the tree is what a test of it asserts.
func TestLowering(t *testing.T) {
	tests := []struct {
		name  string
		built *expression.Call
		want  *expression.Call
	}{
		{
			name:  "named field of a level",
			built: Resource.Service.Eq("myservice"),
			want: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.Reference{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.Scalar{Value: "myservice"},
			}},
		},
		{
			name:  "field named by a variable",
			built: Resource.Field(expression.ResourceFieldSchemaURL).Exists(),
			want: &expression.Call{Op: expression.OpExists, Args: []expression.Expression{
				&expression.Reference{Name: expression.ResourceFieldSchemaURL, Level: expression.LevelResource},
			}},
		},
		{
			name:  "attribute of a level",
			built: Span.Attr("http.route").Ne("/health"),
			want: &expression.Call{Op: expression.OpNe, Args: []expression.Expression{
				&expression.Reference{Name: "http.route", Level: expression.LevelSpan, Attr: true},
				&expression.Scalar{Value: "/health"},
			}},
		},
		{
			name:  "unqualified attribute",
			built: Attr("foo").Eq("bar"),
			want: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.Reference{Name: "foo"},
				&expression.Scalar{Value: "bar"},
			}},
		},
		{
			name:  "regular expression",
			built: Span.Name.Matches("GET|POST"),
			want: &expression.Call{Op: expression.OpRegex, Args: []expression.Expression{
				&expression.Reference{Name: expression.SpanFieldName, Level: expression.LevelSpan},
				&expression.Scalar{Value: "GET|POST"},
			}},
		},
		{
			name:  "existence",
			built: Attr("http.route").Exists(),
			want: &expression.Call{Op: expression.OpExists, Args: []expression.Expression{
				&expression.Reference{Name: "http.route"},
			}},
		},
		{
			name:  "duration passed as a Go duration",
			built: Span.Duration.Gte(2 * time.Second),
			want: &expression.Call{Op: expression.OpGte, Args: []expression.Expression{
				&expression.Reference{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.Scalar{Value: "2s"},
			}},
		},
		{
			name:  "instant passed as a time",
			built: Span.StartTime.Lt(time.Date(2026, 8, 16, 18, 56, 20, 0, time.UTC)),
			want: &expression.Call{Op: expression.OpLt, Args: []expression.Expression{
				&expression.Reference{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.Scalar{Value: "2026-08-16T18:56:20Z"},
			}},
		},
		{
			name:  "membership",
			built: Resource.Service.In("cart", "checkout"),
			want: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.Reference{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.List{Values: []string{"cart", "checkout"}},
			}},
		},
		{
			name:  "exclusion from a typed list",
			built: Attr("http.status_code").NotIn(Values(expression.ValueTypeInt, 500, 503)),
			want: &expression.Call{Op: expression.OpNotIn, Args: []expression.Expression{
				&expression.Reference{Name: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
		},
		{
			name:  "constant of a declared type",
			built: Attr("size").Eq(Value(expression.ValueTypeInt, 4096)),
			want: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.Reference{Name: "size"},
				&expression.Scalar{Value: "4096", Type: expression.ValueTypeInt},
			}},
		},
		{
			name:  "reference against reference",
			built: Span.Attr("retries").Gt(Span.Attr("attempts")),
			want: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.Reference{Name: "retries", Level: expression.LevelSpan, Attr: true},
				&expression.Reference{Name: "attempts", Level: expression.LevelSpan, Attr: true},
			}},
		},
		{
			name:  "negation",
			built: Not(Resource.Service.Eq("healthcheck")),
			want: &expression.Call{Op: expression.OpNot, Args: []expression.Expression{
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.Reference{Name: expression.ResourceFieldService, Level: expression.LevelResource},
					&expression.Scalar{Value: "healthcheck"},
				}},
			}},
		},
		{
			name: "correlated match over the events",
			built: Some(Event, And(
				Event.Name.Eq("exception"),
				Event.TimeSinceStart.Gt(50*time.Microsecond),
			)),
			want: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.Reference{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
					&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
						&expression.Reference{Name: expression.EventFieldName, Level: expression.LevelEvent},
						&expression.Scalar{Value: "exception"},
					}},
					&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
						&expression.Reference{Name: expression.EventFieldTimeSinceStart, Level: expression.LevelEvent},
						&expression.Scalar{Value: "50µs"},
					}},
				}},
			}},
		},
		{
			name:  "correlated match over the links",
			built: Some(Link, Link.TraceID.Eq("4bf92f3577b34da6a3ce929d0e0e4736")),
			want: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.Reference{Level: expression.LevelLink},
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.Reference{Name: expression.LinkFieldTraceID, Level: expression.LevelLink},
					&expression.Scalar{Value: "4bf92f3577b34da6a3ce929d0e0e4736"},
				}},
			}},
		},
		{
			name: "disjunction inside a conjunction",
			built: And(
				Span.Duration.Gt("2s"),
				Or(Attr("a").Eq("1"), Instrumentation.Name.Eq("otelhttp")),
			),
			want: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
					&expression.Reference{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
					&expression.Scalar{Value: "2s"},
				}},
				&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
					&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
						&expression.Reference{Name: "a"},
						&expression.Scalar{Value: "1"},
					}},
					&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
						&expression.Reference{Name: expression.InstrumentationFieldName, Level: expression.LevelInstrumentation},
						&expression.Scalar{Value: "otelhttp"},
					}},
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.built)
			require.NoError(t, expression.ValidateFilter(test.built))
		})
	}
}

// TestNumericComparisonDeclaresTheType covers the one place the builder decides something the
// caller did not say: a numeric comparison against a Go number asks the backend to read the
// value as a number, while equality leaves the type open (RFC 0005 §5.4).
func TestNumericComparisonDeclaresTheType(t *testing.T) {
	tests := []struct {
		name  string
		built *expression.Call
		want  *expression.Scalar
	}{
		{"integer under a comparison", Attr("size").Gt(500), &expression.Scalar{Value: "500", Type: expression.ValueTypeInt}},
		{"unsigned integer", Attr("size").Lte(uint8(7)), &expression.Scalar{Value: "7", Type: expression.ValueTypeInt}},
		{"float", Attr("ratio").Lt(1.5), &expression.Scalar{Value: "1.5", Type: expression.ValueTypeDouble}},
		{"32-bit float", Attr("ratio").Gte(float32(0.5)), &expression.Scalar{Value: "0.5", Type: expression.ValueTypeDouble}},
		{"boolean", Attr("ok").Gt(true), &expression.Scalar{Value: "true", Type: expression.ValueTypeBool}},
		{"duration keeps its unit and no type", Attr("d").Gt(time.Second), &expression.Scalar{Value: "1s"}},
		{"integer under equality", Attr("size").Eq(500), &expression.Scalar{Value: "500"}},
		{"float under equality", Attr("ratio").Eq(1.5), &expression.Scalar{Value: "1.5"}},
		{"boolean under equality", Attr("ok").Eq(false), &expression.Scalar{Value: "false"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.built.Args[1])
		})
	}
}

// TestValueWithoutOwnSpelling covers a value the builder has no case for. It is rendered rather
// than refused, because a constant is carried as a string whatever it holds.
func TestValueWithoutOwnSpelling(t *testing.T) {
	assert.Equal(t,
		&expression.Scalar{Value: "[1 2]"},
		Attr("ids").Eq([]int{1, 2}).Args[1])
	assert.Empty(t, valueTypeOf([]int{1, 2}))
}

// TestCombineFlattens covers the shape And and Or give a tree, which is what a backend limited
// to a flat conjunction has to read.
func TestCombineFlattens(t *testing.T) {
	first := Attr("a").Eq("1")
	second := Attr("b").Eq("2")
	third := Attr("c").Eq("3")

	t.Run("nested and is absorbed", func(t *testing.T) {
		assert.Equal(t,
			&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{first, second, third}},
			And(And(first, second), third))
	})
	t.Run("nested or is left alone under and", func(t *testing.T) {
		assert.Equal(t,
			&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{Or(first, second), third}},
			And(Or(first, second), third))
	})
	t.Run("a lone predicate needs no wrapper", func(t *testing.T) {
		assert.Equal(t, first, And(first))
		assert.Equal(t, first, Or(first))
	})
	t.Run("no predicate is no filter", func(t *testing.T) {
		assert.Nil(t, And())
		assert.Nil(t, Or())
	})
	t.Run("a nil predicate is carried through to be refused", func(t *testing.T) {
		built := And(first, nil)
		require.Len(t, built.Args, 2)
		require.Error(t, expression.ValidateFilter(built))
	})
}

// levelObjects pairs each level with the object naming its fields. The tripwires below look a
// level up here, so a level added to the vocabulary needs an object of its own.
var levelObjects = map[expression.Level]any{
	expression.LevelSpan:            Span,
	expression.LevelResource:        Resource,
	expression.LevelInstrumentation: Instrumentation,
	expression.LevelEvent:           Event,
	expression.LevelLink:            Link,
}

// TestEveryBuiltInFieldHasAnAccessor is a tripwire rather than a property. The named accessors
// spell out a vocabulary that jaeger-idl defines, so a field added there without an accessor
// here has to fail this test rather than leave the builder quietly unable to name it.
func TestEveryBuiltInFieldHasAnAccessor(t *testing.T) {
	for _, field := range expression.Fields() {
		t.Run(string(field.Level)+"."+field.Name, func(t *testing.T) {
			object, ok := levelObjects[field.Level]
			require.True(t, ok, "the %q level has no object naming its fields", field.Level)
			assert.Contains(t, accessors(object),
				&expression.Reference{Name: field.Name, Level: field.Level},
				"add an accessor for this field to the %q level", field.Level)
		})
	}
}

// TestEveryAccessorNamesABuiltInField is the other direction: an accessor spelled the way no
// field is would build a reference the query API refuses.
func TestEveryAccessorNamesABuiltInField(t *testing.T) {
	for level, object := range levelObjects {
		refs := accessors(object)
		require.NotEmpty(t, refs)
		for _, ref := range refs {
			require.NoError(t, expression.ValidateFilter(
				&expression.Call{Op: expression.OpExists, Args: []expression.Expression{ref}},
			))
			assert.Equal(t, level, ref.Level)
		}
	}
}

// accessors reads the references a level object's named accessors hold, so the tripwires walk
// the accessors themselves rather than a list of them repeated in this file.
func accessors(object any) []*expression.Reference {
	value := reflect.ValueOf(object)
	var refs []*expression.Reference
	for i := range value.NumField() {
		if value.Type().Field(i).Type != reflect.TypeOf(Ref{}) {
			continue
		}
		refs = append(refs, value.Field(i).Interface().(Ref).ref)
	}
	return refs
}
