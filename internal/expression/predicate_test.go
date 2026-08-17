// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// p is the builder under test. Predicate holds no state, so one value serves every case.
var p Predicate

// TestLowering pins what each chain lowers to. The builder promises no more than the tree it
// writes, so the tree is what a test of it asserts.
func TestLowering(t *testing.T) {
	tests := []struct {
		name  string
		built *ast.Call
		want  *ast.Call
	}{
		{
			name:  "named field of a level",
			built: p.Resource().Service.Eq("myservice"),
			want: &ast.Call{Op: ast.OpEq, Args: []ast.Expression{
				&ast.FieldRef{Name: ast.ResourceFieldService, Level: ast.LevelResource},
				&ast.AnyValue{Value: "myservice"},
			}},
		},
		{
			name:  "field named by a variable",
			built: p.Resource().Field(ast.ResourceFieldSchemaURL).Exists(),
			want: &ast.Call{Op: ast.OpExists, Args: []ast.Expression{
				&ast.FieldRef{Name: ast.ResourceFieldSchemaURL, Level: ast.LevelResource},
			}},
		},
		{
			name:  "attribute of a level",
			built: p.Span().Attr("http.route").Ne("/health"),
			want: &ast.Call{Op: ast.OpNe, Args: []ast.Expression{
				&ast.AttributeRef{Key: "http.route", Level: ast.LevelSpan},
				&ast.AnyValue{Value: "/health"},
			}},
		},
		{
			name:  "unqualified attribute",
			built: p.Attr("foo").Eq("bar"),
			want: &ast.Call{Op: ast.OpEq, Args: []ast.Expression{
				&ast.AttributeRef{Key: "foo"},
				&ast.AnyValue{Value: "bar"},
			}},
		},
		{
			name:  "regular expression",
			built: p.Span().Name.Matches("GET|POST"),
			want: &ast.Call{Op: ast.OpRegex, Args: []ast.Expression{
				&ast.FieldRef{Name: ast.SpanFieldName, Level: ast.LevelSpan},
				&ast.AnyValue{Value: "GET|POST"},
			}},
		},
		{
			name:  "existence",
			built: p.Attr("http.route").Exists(),
			want: &ast.Call{Op: ast.OpExists, Args: []ast.Expression{
				&ast.AttributeRef{Key: "http.route"},
			}},
		},
		{
			name:  "duration passed as a Go duration",
			built: p.Span().Duration.Gte(2 * time.Second),
			want: &ast.Call{Op: ast.OpGte, Args: []ast.Expression{
				&ast.FieldRef{Name: ast.SpanFieldDuration, Level: ast.LevelSpan},
				&ast.DurationValue{Value: 2 * time.Second},
			}},
		},
		{
			name:  "instant passed as a time",
			built: p.Span().StartTime.Lt(time.Date(2026, 8, 16, 18, 56, 20, 0, time.UTC)),
			want: &ast.Call{Op: ast.OpLt, Args: []ast.Expression{
				&ast.FieldRef{Name: ast.SpanFieldStartTime, Level: ast.LevelSpan},
				&ast.TimestampValue{Value: time.Date(2026, 8, 16, 18, 56, 20, 0, time.UTC)},
			}},
		},
		{
			name:  "membership",
			built: p.Resource().Service.In("cart", "checkout"),
			want: &ast.Call{Op: ast.OpIn, Args: []ast.Expression{
				&ast.FieldRef{Name: ast.ResourceFieldService, Level: ast.LevelResource},
				&ast.List{Values: []string{"cart", "checkout"}},
			}},
		},
		{
			name:  "exclusion from a typed list",
			built: p.Attr("http.status_code").NotIn(p.List(ast.ValueTypeInt, 500, 503)),
			want: &ast.Call{Op: ast.OpNotIn, Args: []ast.Expression{
				&ast.AttributeRef{Key: "http.status_code"},
				&ast.List{Values: []string{"500", "503"}, Type: ast.ValueTypeInt},
			}},
		},
		{
			name:  "constant narrowed to text",
			built: p.Attr("size").Eq(p.Text("4096")),
			want: &ast.Call{Op: ast.OpEq, Args: []ast.Expression{
				&ast.AttributeRef{Key: "size"},
				&ast.StringValue{Value: "4096"},
			}},
		},
		{
			name:  "reference against reference",
			built: p.Span().Attr("retries").Gt(p.Span().Attr("attempts")),
			want: &ast.Call{Op: ast.OpGt, Args: []ast.Expression{
				&ast.AttributeRef{Key: "retries", Level: ast.LevelSpan},
				&ast.AttributeRef{Key: "attempts", Level: ast.LevelSpan},
			}},
		},
		{
			name:  "negation",
			built: p.Not(p.Resource().Service.Eq("healthcheck")),
			want: &ast.Call{Op: ast.OpNot, Args: []ast.Expression{
				&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
					&ast.FieldRef{Name: ast.ResourceFieldService, Level: ast.LevelResource},
					&ast.AnyValue{Value: "healthcheck"},
				}},
			}},
		},
		{
			name: "correlated match over the events",
			built: p.Some(p.Event(), p.And(
				p.Event().Name.Eq("exception"),
				p.Event().TimeSinceStart.Gt(50*time.Microsecond),
			)),
			want: &ast.Call{Op: ast.OpSome, Args: []ast.Expression{
				&ast.NestedRef{Level: ast.LevelEvent},
				&ast.Call{Op: ast.OpAnd, Args: []ast.Expression{
					&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
						&ast.FieldRef{Name: ast.EventFieldName, Level: ast.LevelEvent},
						&ast.AnyValue{Value: "exception"},
					}},
					&ast.Call{Op: ast.OpGt, Args: []ast.Expression{
						&ast.FieldRef{Name: ast.EventFieldTimeSinceStart, Level: ast.LevelEvent},
						&ast.DurationValue{Value: 50 * time.Microsecond},
					}},
				}},
			}},
		},
		{
			name:  "correlated match over the links",
			built: p.Some(p.Link(), p.Link().TraceID.Eq("4bf92f3577b34da6a3ce929d0e0e4736")),
			want: &ast.Call{Op: ast.OpSome, Args: []ast.Expression{
				&ast.NestedRef{Level: ast.LevelLink},
				&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
					&ast.FieldRef{Name: ast.LinkFieldTraceID, Level: ast.LevelLink},
					&ast.AnyValue{Value: "4bf92f3577b34da6a3ce929d0e0e4736"},
				}},
			}},
		},
		{
			name: "disjunction inside a conjunction",
			built: p.And(
				p.Span().Duration.Gt("2s"),
				p.Or(p.Attr("a").Eq("1"), p.Scope().Name.Eq("otelhttp")),
			),
			want: &ast.Call{Op: ast.OpAnd, Args: []ast.Expression{
				&ast.Call{Op: ast.OpGt, Args: []ast.Expression{
					&ast.FieldRef{Name: ast.SpanFieldDuration, Level: ast.LevelSpan},
					&ast.AnyValue{Value: "2s"},
				}},
				&ast.Call{Op: ast.OpOr, Args: []ast.Expression{
					&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
						&ast.AttributeRef{Key: "a"},
						&ast.AnyValue{Value: "1"},
					}},
					&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
						&ast.FieldRef{Name: ast.ScopeFieldName, Level: ast.LevelScope},
						&ast.AnyValue{Value: "otelhttp"},
					}},
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.built)
			require.NoError(t, ast.ValidateFilter(test.built))
		})
	}
}

// TestCompareTakesTheOperatorAtRunTime covers the prefix form, which exists for an operator the
// caller does not know when writing the query. It has to agree with the named methods, and the
// membership operators have to take a list rather than a scalar operand.
func TestCompareTakesTheOperatorAtRunTime(t *testing.T) {
	ref := p.Resource().Service

	assert.Equal(t, ref.Eq("cart"), p.Compare(ast.OpEq, ref, "cart"))
	assert.Equal(t, ref.Gte(2*time.Second), p.Compare(ast.OpGte, ref, 2*time.Second))
	assert.Equal(t, ref.Matches("ca.*"), p.Compare(ast.OpRegex, ref, "ca.*"))
	assert.Equal(t, ref.In("cart"), p.Compare(ast.OpIn, ref, "cart"))
	assert.Equal(t, ref.NotIn(p.List(ast.ValueTypeInt, 1)), p.Compare(ast.OpNotIn, ref, p.List(ast.ValueTypeInt, 1)))
}

// TestConstantCarriesTheGoType covers the node a Go value is compared as. The AST holds a typed
// constant, so the builder writes the type the caller already stated by choosing a Go type; only
// a string leaves it open, which is what matches a value in whatever form it was stored.
func TestConstantCarriesTheGoType(t *testing.T) {
	tests := []struct {
		name  string
		built *ast.Call
		want  ast.Expression
	}{
		{"integer", p.Attr("size").Gt(500), &ast.IntValue{Value: 500}},
		{"unsigned integer", p.Attr("size").Lte(uint8(7)), &ast.IntValue{Value: 7}},
		{"unsigned integer past the signed range", p.Attr("size").Lte(uint64(math.MaxUint64)), &ast.AnyValue{Value: "18446744073709551615"}},
		{"float", p.Attr("ratio").Lt(1.5), &ast.DoubleValue{Value: 1.5}},
		{"32-bit float", p.Attr("ratio").Gte(float32(0.5)), &ast.DoubleValue{Value: 0.5}},
		{"boolean", p.Attr("ok").Gt(true), &ast.BoolValue{Value: true}},
		{"duration", p.Attr("d").Gt(time.Second), &ast.DurationValue{Value: time.Second}},
		{"instant", p.Attr("t").Gt(time.Unix(0, 0).UTC()), &ast.TimestampValue{Value: time.Unix(0, 0).UTC()}},

		// Equality and membership leave the type open, whatever Go type carried the value, so a
		// key stored as text still matches (RFC 0005 §5.4). A duration keeps the syntax
		// ResolveConstants reads it back from.
		{"an integer under equality", p.Attr("size").Eq(500), &ast.AnyValue{Value: "500"}},
		{"a boolean under equality", p.Attr("ok").Eq(true), &ast.AnyValue{Value: "true"}},
		{"a duration under equality", p.Attr("d").Eq(time.Second), &ast.AnyValue{Value: "1s"}},
		{"a string is always open", p.Attr("size").Eq("500"), &ast.AnyValue{Value: "500"}},
		{"an instant under equality", p.Attr("t").Eq(time.Unix(0, 0).UTC()), &ast.AnyValue{Value: "1970-01-01T00:00:00Z"}},
		{"a value with no Go spelling of its own", p.Attr("ids").Eq([]int{1, 2}), &ast.AnyValue{Value: "[1 2]"}},
		{"a string under an ordered comparison", p.Attr("v").Gt("1.2.3"), &ast.AnyValue{Value: "1.2.3"}},
		{"a value of no filter type at all", p.Attr("ids").Gt([]int{1, 2}), &ast.AnyValue{Value: "[1 2]"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.built.Args[1])
		})
	}
}

// TestListElementSpelling covers how a Go value is written into a list, whose elements stay
// spellings because that is what the AST holds them as.
func TestListElementSpelling(t *testing.T) {
	instant := time.Date(2026, 8, 16, 18, 56, 20, 0, time.UTC)
	assert.Equal(t,
		&ast.List{Values: []string{
			"cart", "500", "1.5", "0.5", "true", "2s", "2026-08-16T18:56:20Z",
		}},
		p.Attr("k").In("cart", 500, 1.5, float32(0.5), true, 2*time.Second, instant).Args[1])
}

// TestValueWithoutOwnSpelling covers a value the builder has no case for. It is rendered rather
// than refused, because a constant of no declared type carries a spelling whatever it holds.
func TestValueWithoutOwnSpelling(t *testing.T) {
	assert.Equal(t,
		&ast.AnyValue{Value: "[1 2]"},
		p.Attr("ids").Eq([]int{1, 2}).Args[1])
	assert.Equal(t,
		&ast.List{Values: []string{"[1 2]"}},
		p.Attr("ids").In([]int{1, 2}).Args[1])
}

// TestCombineFlattens covers the shape And and Or give a tree, which is what a backend limited
// to a flat conjunction has to read.
func TestCombineFlattens(t *testing.T) {
	first := p.Attr("a").Eq("1")
	second := p.Attr("b").Eq("2")
	third := p.Attr("c").Eq("3")

	t.Run("nested and is absorbed", func(t *testing.T) {
		assert.Equal(t,
			&ast.Call{Op: ast.OpAnd, Args: []ast.Expression{first, second, third}},
			p.And(p.And(first, second), third))
	})
	t.Run("nested or is left alone under and", func(t *testing.T) {
		assert.Equal(t,
			&ast.Call{Op: ast.OpAnd, Args: []ast.Expression{p.Or(first, second), third}},
			p.And(p.Or(first, second), third))
	})
	t.Run("a lone predicate needs no wrapper", func(t *testing.T) {
		assert.Equal(t, first, p.And(first))
		assert.Equal(t, first, p.Or(first))
	})
	t.Run("no predicate is no filter", func(t *testing.T) {
		assert.Nil(t, p.And())
		assert.Nil(t, p.Or())
	})
	t.Run("a nil predicate is carried through to be refused", func(t *testing.T) {
		built := p.And(first, nil)
		require.Len(t, built.Args, 2)
		require.Error(t, ast.ValidateFilter(built))
	})
}

// levelObjects pairs each level with the object naming its fields. The tripwires below look a
// level up here, so a level added to the vocabulary needs an object of its own.
var levelObjects = map[ast.Level]any{
	ast.LevelSpan:     p.Span(),
	ast.LevelResource: p.Resource(),
	ast.LevelScope:    p.Scope(),
	ast.LevelEvent:    p.Event(),
	ast.LevelLink:     p.Link(),
}

// TestEveryBuiltInFieldHasAnAccessor is a tripwire rather than a property. The named accessors
// spell out a vocabulary that jaeger-idl defines, so a field added there without an accessor
// here has to fail this test rather than leave the builder quietly unable to name it.
func TestEveryBuiltInFieldHasAnAccessor(t *testing.T) {
	for _, field := range ast.Fields() {
		t.Run(string(field.Level)+"."+field.Name, func(t *testing.T) {
			object, ok := levelObjects[field.Level]
			require.True(t, ok, "the %q level has no object naming its fields", field.Level)
			assert.Contains(t, accessors(t, object),
				&ast.FieldRef{Name: field.Name, Level: field.Level},
				"add an accessor for this field to the %q level", field.Level)
		})
	}
}

// TestEveryAccessorNamesABuiltInField is the other direction: an accessor spelled the way no
// field is would build a reference the query API refuses.
func TestEveryAccessorNamesABuiltInField(t *testing.T) {
	for level, object := range levelObjects {
		refs := accessors(t, object)
		require.NotEmpty(t, refs)
		for _, ref := range refs {
			require.NoError(t, ast.ValidateFilter(
				&ast.Call{Op: ast.OpExists, Args: []ast.Expression{ref}},
			))
			assert.Equal(t, level, ref.Level)
		}
	}
}

// accessors reads the references a level object's named accessors hold, so the tripwires walk
// the accessors themselves rather than a list of them repeated in this file. Every one of them
// names a built-in field, which is what makes the field reference the type to read them as.
func accessors(t *testing.T, object any) []*ast.FieldRef {
	value := reflect.ValueOf(object)
	var refs []*ast.FieldRef
	for i := range value.NumField() {
		if value.Type().Field(i).Type != reflect.TypeOf(Ref{}) {
			continue
		}
		ref, ok := value.Field(i).Interface().(Ref).ref.(*ast.FieldRef)
		require.True(t, ok, "accessor %q does not name a built-in field", value.Type().Field(i).Name)
		refs = append(refs, ref)
	}
	return refs
}
