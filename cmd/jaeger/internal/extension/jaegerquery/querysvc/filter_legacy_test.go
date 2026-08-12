// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func attributes(pairs map[string]string) pcommon.Map {
	m := pcommon.NewMap()
	for k, v := range pairs {
		m.PutStr(k, v)
	}
	return m
}

// TestPrepareFilter_RewritesAsLegacyFields covers the backends that do not evaluate a
// filter yet: the query service expresses what it can in the predicate fields they already
// serve, so a filter query returns what the equivalent legacy query would.
func TestPrepareFilter_RewritesAsLegacyFields(t *testing.T) {
	tests := []struct {
		name     string
		filter   *tracestore.Call
		expected tracestore.TraceQueryParams
	}{
		{
			name:     "an unqualified attribute becomes a tag",
			filter:   predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.status_code"}, "500"),
			expected: tracestore.TraceQueryParams{Attributes: attributes(map[string]string{"http.status_code": "500"})},
		},
		{
			name: "a level-qualified attribute widens to an unqualified tag",
			filter: predicate(tracestore.OpEq,
				&tracestore.Reference{Name: "k8s.pod.name", Level: tracestore.LevelResource, Attr: true}, "cart-0"),
			expected: tracestore.TraceQueryParams{Attributes: attributes(map[string]string{"k8s.pod.name": "cart-0"})},
		},
		{
			name: "an event-level attribute widens too",
			filter: predicate(tracestore.OpEq,
				&tracestore.Reference{Name: "exception.type", Level: tracestore.LevelEvent, Attr: true}, "IOError"),
			expected: tracestore.TraceQueryParams{Attributes: attributes(map[string]string{"exception.type": "IOError"})},
		},
		{
			name: "the service built-in becomes the service name",
			filter: predicate(tracestore.OpEq,
				tracestore.ResourceService.Ref(), "cart"),
			expected: tracestore.TraceQueryParams{ServiceName: "cart", Attributes: pcommon.NewMap()},
		},
		{
			name: "the span name built-in becomes the operation name",
			filter: predicate(tracestore.OpEq,
				tracestore.SpanName.Ref(), "GET /cart"),
			expected: tracestore.TraceQueryParams{OperationName: "GET /cart", Attributes: pcommon.NewMap()},
		},
		{
			name: "the inclusive duration bounds become the duration range",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpGte, tracestore.SpanDuration.Ref(), "2s"),
				predicate(tracestore.OpLte, tracestore.SpanDuration.Ref(), "1m30s"),
			}},
			expected: tracestore.TraceQueryParams{
				DurationMin: 2 * time.Second,
				DurationMax: 90 * time.Second,
				Attributes:  pcommon.NewMap(),
			},
		},
		{
			name: "a conjunction fills several fields at once",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpEq, tracestore.ResourceService.Ref(), "cart"),
				predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.method"}, "GET"),
				predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.status_code"}, "500"),
			}},
			expected: tracestore.TraceQueryParams{
				ServiceName: "cart",
				Attributes:  attributes(map[string]string{"http.method": "GET", "http.status_code": "500"}),
			},
		},
		{
			// `and` is associative, so a nested conjunction asks the same question as a flat one
			// and is flattened rather than refused.
			name: "a conjunction nested in a conjunction",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpEq, tracestore.ResourceService.Ref(), "cart"),
				&tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
					predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.method"}, "GET"),
					&tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
						predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.status_code"}, "500"),
						predicate(tracestore.OpGte, tracestore.SpanDuration.Ref(), "2s"),
					}},
				}},
			}},
			expected: tracestore.TraceQueryParams{
				ServiceName: "cart",
				DurationMin: 2 * time.Second,
				Attributes:  attributes(map[string]string{"http.method": "GET", "http.status_code": "500"}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := prepareFilter(filterQuery(test.filter), tracestore.SearchCapabilities{})
			require.NoError(t, err)
			assert.Nil(t, got.Filter, "the filter is consumed, so the reader sees one filtering model")
			// Attributes are compared by content, because the map is a set of tag filters and the
			// order the rewrite inserts them in is not part of what a reader is promised.
			assert.Equal(t, test.expected.Attributes.AsRaw(), got.Attributes.AsRaw())
			expected, actual := test.expected, got.TraceQueryParams
			expected.Attributes, actual.Attributes = pcommon.NewMap(), pcommon.NewMap()
			assert.Equal(t, expected, actual)
		})
	}
}

// TestPrepareFilter_RefusesWhatLegacyFieldsCannotExpress covers the other half of the
// contract: a predicate the legacy fields cannot carry is refused rather than widened into
// a different question.
func TestPrepareFilter_RefusesWhatLegacyFieldsCannotExpress(t *testing.T) {
	eqRef := func(name string) *tracestore.Call {
		return predicate(tracestore.OpEq, &tracestore.Reference{Name: name}, "1")
	}
	tests := []struct {
		name        string
		filter      *tracestore.Call
		expectedErr string
	}{
		{
			name: "a disjunction",
			filter: &tracestore.Call{Op: tracestore.OpOr, Args: []tracestore.Expression{
				eqRef("a"), eqRef("b"),
			}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name:        "a negation",
			filter:      &tracestore.Call{Op: tracestore.OpNot, Args: []tracestore.Expression{eqRef("a")}},
			expectedErr: `does not support the operator "not"`,
		},
		{
			name: "a disjunction nested in a conjunction",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				eqRef("a"),
				&tracestore.Call{Op: tracestore.OpOr, Args: []tracestore.Expression{eqRef("b"), eqRef("c")}},
			}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a bare reference among the conjuncts",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				eqRef("a"), &tracestore.Reference{Name: "b"},
			}},
			expectedErr: "conjunction of predicates only",
		},
		{
			name:        "an inequality on an attribute",
			filter:      predicate(tracestore.OpNe, &tracestore.Reference{Name: "http.method"}, "GET"),
			expectedErr: `does not support the operator "ne" on "http.method"`,
		},
		{
			name: "an exclusive duration bound",
			filter: predicate(tracestore.OpGt,
				tracestore.SpanDuration.Ref(), "2s"),
			expectedErr: `does not support the operator "gt" on "duration"`,
		},
		{
			name: "a regex on the operation name",
			filter: predicate(tracestore.OpRegex,
				tracestore.SpanName.Ref(), "GET .*"),
			expectedErr: `does not support the operator "regex" on "name"`,
		},
		{
			name: "an inequality on the service",
			filter: predicate(tracestore.OpNe,
				tracestore.ResourceService.Ref(), "cart"),
			expectedErr: `does not support the operator "ne" on "service"`,
		},
		{
			name: "an attribute at a level no backend indexes",
			filter: predicate(tracestore.OpEq,
				&tracestore.Reference{Name: "peer.service", Level: tracestore.LevelLink, Attr: true}, "cart"),
			expectedErr: `does not index the "link" level`,
		},
		{
			name: "an instrumentation-level attribute",
			filter: predicate(tracestore.OpEq,
				&tracestore.Reference{Name: "lib", Level: tracestore.LevelInstrumentation, Attr: true}, "otel"),
			expectedErr: `does not index the "instrumentation" level`,
		},
		{
			name: "a built-in field with no legacy equivalent",
			filter: predicate(tracestore.OpEq,
				&tracestore.Reference{Name: "kind", Level: tracestore.LevelSpan}, "server"),
			expectedErr: `does not support the built-in field "kind" of the "span" level`,
		},
		{
			name: "set membership",
			filter: &tracestore.Call{Op: tracestore.OpIn, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "http.status_code"},
				&tracestore.List{Values: []string{"500", "503"}},
			}},
			expectedErr: "compares a reference against a constant only",
		},
		{
			name: "a comparison of two references",
			filter: &tracestore.Call{Op: tracestore.OpNe, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "enduser.id", Level: tracestore.LevelSpan, Attr: true},
				&tracestore.Reference{Name: "enduser.id", Level: tracestore.LevelResource, Attr: true},
			}},
			expectedErr: "compares a reference against a constant only",
		},
		{
			name: "an existence test",
			filter: &tracestore.Call{Op: tracestore.OpExists, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "error"},
			}},
			expectedErr: `does not support the operator "exists"`,
		},
		{
			name: "two predicates on the same attribute",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpEq, &tracestore.Reference{Name: "a"}, "1"),
				predicate(tracestore.OpEq, &tracestore.Reference{Name: "a"}, "2"),
			}},
			expectedErr: `only one predicate on "a"`,
		},
		{
			name: "two services",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpEq, tracestore.ResourceService.Ref(), "cart"),
				predicate(tracestore.OpEq, tracestore.ResourceService.Ref(), "checkout"),
			}},
			expectedErr: `only one predicate on "service"`,
		},
		{
			name: "two operation names",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpEq, tracestore.SpanName.Ref(), "a"),
				predicate(tracestore.OpEq, tracestore.SpanName.Ref(), "b"),
			}},
			expectedErr: `only one predicate on "name"`,
		},
		{
			name: "two lower duration bounds",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpGte, tracestore.SpanDuration.Ref(), "1s"),
				predicate(tracestore.OpGte, tracestore.SpanDuration.Ref(), "2s"),
			}},
			expectedErr: `only one predicate on "duration"`,
		},
		{
			name: "two upper duration bounds",
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				predicate(tracestore.OpLte, tracestore.SpanDuration.Ref(), "1s"),
				predicate(tracestore.OpLte, tracestore.SpanDuration.Ref(), "2s"),
			}},
			expectedErr: `only one predicate on "duration"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := filterQuery(test.filter)
			got, err := prepareFilter(query, tracestore.SearchCapabilities{})
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), test.expectedErr)
			assert.Equal(t, query, got, "the query is returned unchanged so nothing half-rewritten reaches storage")
		})
	}
}

// TestPrepareFilter_RefusesAnUnparsableDuration covers the one mistake structural
// validation cannot catch, because the filter AST does not carry types.
func TestPrepareFilter_RefusesAnUnparsableDuration(t *testing.T) {
	filter := predicate(tracestore.OpGte,
		tracestore.SpanDuration.Ref(), "quickly")
	_, err := prepareFilter(filterQuery(filter), tracestore.SearchCapabilities{})
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.Contains(t, err.Error(), `"quickly" is not a duration`)
}
