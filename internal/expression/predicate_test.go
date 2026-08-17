// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
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
				&ast.Reference{Name: ast.ResourceFieldService, Level: ast.LevelResource},
				&ast.Scalar{Value: "myservice"},
			}},
		},
		{
			name:  "field named by a variable",
			built: p.Resource().Field(ast.ResourceFieldSchemaURL).Exists(),
			want: &ast.Call{Op: ast.OpExists, Args: []ast.Expression{
				&ast.Reference{Name: ast.ResourceFieldSchemaURL, Level: ast.LevelResource},
			}},
		},
		{
			name:  "attribute of a level",
			built: p.Span().Attr("http.route").Ne("/health"),
			want: &ast.Call{Op: ast.OpNe, Args: []ast.Expression{
				&ast.Reference{Name: "http.route", Level: ast.LevelSpan, Attr: true},
				&ast.Scalar{Value: "/health"},
			}},
		},
		{
			name:  "unqualified attribute",
			built: p.Attr("foo").Eq("bar"),
			want: &ast.Call{Op: ast.OpEq, Args: []ast.Expression{
				&ast.Reference{Name: "foo"},
				&ast.Scalar{Value: "bar"},
			}},
		},
		{
			name:  "regular expression",
			built: p.Span().Name.Matches("GET|POST"),
			want: &ast.Call{Op: ast.OpRegex, Args: []ast.Expression{
				&ast.Reference{Name: ast.SpanFieldName, Level: ast.LevelSpan},
				&ast.Scalar{Value: "GET|POST"},
			}},
		},
		{
			name:  "existence",
			built: p.Attr("http.route").Exists(),
			want: &ast.Call{Op: ast.OpExists, Args: []ast.Expression{
				&ast.Reference{Name: "http.route"},
			}},
		},
		{
			name:  "duration passed as a Go duration",
			built: p.Span().Duration.Gte(2 * time.Second),
			want: &ast.Call{Op: ast.OpGte, Args: []ast.Expression{
				&ast.Reference{Name: ast.SpanFieldDuration, Level: ast.LevelSpan},
				&ast.Scalar{Value: "2s"},
			}},
		},
		{
			name:  "instant passed as a time",
			built: p.Span().StartTime.Lt(time.Date(2026, 8, 16, 18, 56, 20, 0, time.UTC)),
			want: &ast.Call{Op: ast.OpLt, Args: []ast.Expression{
				&ast.Reference{Name: ast.SpanFieldStartTime, Level: ast.LevelSpan},
				&ast.Scalar{Value: "2026-08-16T18:56:20Z"},
			}},
		},
		{
			name:  "membership",
			built: p.Resource().Service.In("cart", "checkout"),
			want: &ast.Call{Op: ast.OpIn, Args: []ast.Expression{
				&ast.Reference{Name: ast.ResourceFieldService, Level: ast.LevelResource},
				&ast.List{Values: []string{"cart", "checkout"}},
			}},
		},
		{
			name:  "exclusion from a typed list",
			built: p.Attr("http.status_code").NotIn(p.List(ast.ValueTypeInt, 500, 503)),
			want: &ast.Call{Op: ast.OpNotIn, Args: []ast.Expression{
				&ast.Reference{Name: "http.status_code"},
				&ast.List{Values: []string{"500", "503"}, Type: ast.ValueTypeInt},
			}},
		},
		{
			name:  "constant of a declared type",
			built: p.Attr("size").Eq(p.Scalar(ast.ValueTypeInt, 4096)),
			want: &ast.Call{Op: ast.OpEq, Args: []ast.Expression{
				&ast.Reference{Name: "size"},
				&ast.Scalar{Value: "4096", Type: ast.ValueTypeInt},
			}},
		},
		{
			name:  "reference against reference",
			built: p.Span().Attr("retries").Gt(p.Span().Attr("attempts")),
			want: &ast.Call{Op: ast.OpGt, Args: []ast.Expression{
				&ast.Reference{Name: "retries", Level: ast.LevelSpan, Attr: true},
				&ast.Reference{Name: "attempts", Level: ast.LevelSpan, Attr: true},
			}},
		},
		{
			name:  "negation",
			built: p.Not(p.Resource().Service.Eq("healthcheck")),
			want: &ast.Call{Op: ast.OpNot, Args: []ast.Expression{
				&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
					&ast.Reference{Name: ast.ResourceFieldService, Level: ast.LevelResource},
					&ast.Scalar{Value: "healthcheck"},
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
				&ast.Reference{Level: ast.LevelEvent},
				&ast.Call{Op: ast.OpAnd, Args: []ast.Expression{
					&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
						&ast.Reference{Name: ast.EventFieldName, Level: ast.LevelEvent},
						&ast.Scalar{Value: "exception"},
					}},
					&ast.Call{Op: ast.OpGt, Args: []ast.Expression{
						&ast.Reference{Name: ast.EventFieldTimeSinceStart, Level: ast.LevelEvent},
						&ast.Scalar{Value: "50µs"},
					}},
				}},
			}},
		},
		{
			name:  "correlated match over the links",
			built: p.Some(p.Link(), p.Link().TraceID.Eq("4bf92f3577b34da6a3ce929d0e0e4736")),
			want: &ast.Call{Op: ast.OpSome, Args: []ast.Expression{
				&ast.Reference{Level: ast.LevelLink},
				&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
					&ast.Reference{Name: ast.LinkFieldTraceID, Level: ast.LevelLink},
					&ast.Scalar{Value: "4bf92f3577b34da6a3ce929d0e0e4736"},
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
					&ast.Reference{Name: ast.SpanFieldDuration, Level: ast.LevelSpan},
					&ast.Scalar{Value: "2s"},
				}},
				&ast.Call{Op: ast.OpOr, Args: []ast.Expression{
					&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
						&ast.Reference{Name: "a"},
						&ast.Scalar{Value: "1"},
					}},
					&ast.Call{Op: ast.OpEq, Args: []ast.Expression{
						&ast.Reference{Name: ast.ScopeFieldName, Level: ast.LevelScope},
						&ast.Scalar{Value: "otelhttp"},
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

// TestNumericComparisonDeclaresTheType covers the one place the builder decides something the
// caller did not say: a numeric comparison against a Go number asks the backend to read the
// value as a number, while equality leaves the type open (RFC 0005 §5.4).
func TestNumericComparisonDeclaresTheType(t *testing.T) {
	tests := []struct {
		name  string
		built *ast.Call
		want  *ast.Scalar
	}{
		{"integer under a comparison", p.Attr("size").Gt(500), &ast.Scalar{Value: "500", Type: ast.ValueTypeInt}},
		{"unsigned integer", p.Attr("size").Lte(uint8(7)), &ast.Scalar{Value: "7", Type: ast.ValueTypeInt}},
		{"float", p.Attr("ratio").Lt(1.5), &ast.Scalar{Value: "1.5", Type: ast.ValueTypeDouble}},
		{"32-bit float", p.Attr("ratio").Gte(float32(0.5)), &ast.Scalar{Value: "0.5", Type: ast.ValueTypeDouble}},
		{"boolean", p.Attr("ok").Gt(true), &ast.Scalar{Value: "true", Type: ast.ValueTypeBool}},
		{"duration keeps its unit and no type", p.Attr("d").Gt(time.Second), &ast.Scalar{Value: "1s"}},
		{"integer under equality", p.Attr("size").Eq(500), &ast.Scalar{Value: "500"}},
		{"float under equality", p.Attr("ratio").Eq(1.5), &ast.Scalar{Value: "1.5"}},
		{"boolean under equality", p.Attr("ok").Eq(false), &ast.Scalar{Value: "false"}},
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
		&ast.Scalar{Value: "[1 2]"},
		p.Attr("ids").Eq([]int{1, 2}).Args[1])
	assert.Empty(t, valueTypeOf([]int{1, 2}))
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
			assert.Contains(t, accessors(object),
				&ast.Reference{Name: field.Name, Level: field.Level},
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
			require.NoError(t, ast.ValidateFilter(
				&ast.Call{Op: ast.OpExists, Args: []ast.Expression{ref}},
			))
			assert.Equal(t, level, ref.Level)
		}
	}
}

// accessors reads the references a level object's named accessors hold, so the tripwires walk
// the accessors themselves rather than a list of them repeated in this file.
func accessors(object any) []*ast.Reference {
	value := reflect.ValueOf(object)
	var refs []*ast.Reference
	for i := range value.NumField() {
		if value.Type().Field(i).Type != reflect.TypeOf(Ref{}) {
			continue
		}
		refs = append(refs, value.Field(i).Interface().(Ref).ref)
	}
	return refs
}
