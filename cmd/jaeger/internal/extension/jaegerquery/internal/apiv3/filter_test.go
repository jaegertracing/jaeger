// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
)

// enableStructuredFilters turns the filter feature gate on for one test and restores it
// afterwards. The gate is off by default, so every test that sends a filter through the
// api_v3 edge has to ask for it — which is the isolation the gate exists to give a deployment
// that has not opted in.
func enableStructuredFilters(t *testing.T) {
	original := structuredFiltersGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(structuredFiltersGate.ID(), true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(structuredFiltersGate.ID(), original))
	})
}

func protoRef(ref *expressionproto.Reference) *expressionproto.Expression {
	return &expressionproto.Expression{Term: &expressionproto.Expression_Ref{Ref: ref}}
}

func protoScalar(value string, valueType string) *expressionproto.Expression {
	return &expressionproto.Expression{Term: &expressionproto.Expression_Scalar{Scalar: &expressionproto.Scalar{Value: value, Type: valueType}}}
}

func protoCall(call *expressionproto.Call) *expressionproto.Expression {
	return &expressionproto.Expression{Term: &expressionproto.Expression_Call{Call: call}}
}

func TestToStorageFilter_NoFilter(t *testing.T) {
	filter, err := toFilter(nil)
	require.NoError(t, err)
	assert.Nil(t, filter)
}

func TestToStorageFilter_Converts(t *testing.T) {
	tests := []struct {
		name     string
		proto    *expressionproto.Call
		expected *expression.Call
	}{
		{
			name: "an unqualified attribute equality",
			proto: &expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{
				protoRef(&expressionproto.Reference{Name: "http.status_code"}),
				protoScalar("500", ""),
			}},
			expected: &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.Reference{Name: "http.status_code"},
				&expression.Scalar{Value: "500"},
			}},
		},
		{
			name: "a level-qualified attribute and a typed constant",
			proto: &expressionproto.Call{Op: "gt", Args: []*expressionproto.Expression{
				protoRef(&expressionproto.Reference{Name: "size", Level: "span", Attr: true}),
				protoScalar("500", "int"),
			}},
			expected: &expression.Call{Op: expression.OpGt, Args: []expression.Expression{
				&expression.Reference{Name: "size", Level: expression.LevelSpan, Attr: true},
				&expression.Scalar{Value: "500", Type: expression.ValueTypeInt},
			}},
		},
		{
			name: "a list operand",
			proto: &expressionproto.Call{Op: "in", Args: []*expressionproto.Expression{
				protoRef(&expressionproto.Reference{Name: "http.status_code"}),
				{Term: &expressionproto.Expression_List{List: &expressionproto.List{Values: []string{"500", "503"}, Type: "int"}}},
			}},
			expected: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.Reference{Name: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}, Type: expression.ValueTypeInt},
			}},
		},
		{
			name: "a nested boolean tree",
			proto: &expressionproto.Call{Op: "and", Args: []*expressionproto.Expression{
				protoCall(&expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{
					protoRef(&expressionproto.Reference{Name: "a"}), protoScalar("1", ""),
				}}),
				protoCall(&expressionproto.Call{Op: "or", Args: []*expressionproto.Expression{
					protoCall(&expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{
						protoRef(&expressionproto.Reference{Name: "b"}), protoScalar("2", ""),
					}}),
					protoCall(&expressionproto.Call{Op: "exists", Args: []*expressionproto.Expression{
						protoRef(&expressionproto.Reference{Name: "c"}),
					}}),
				}}),
			}},
			expected: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.Reference{Name: "a"}, &expression.Scalar{Value: "1"},
				}},
				&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
					&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
						&expression.Reference{Name: "b"}, &expression.Scalar{Value: "2"},
					}},
					&expression.Call{Op: expression.OpExists, Args: []expression.Expression{
						&expression.Reference{Name: "c"},
					}},
				}},
			}},
		},
		{
			name: "a correlated match over the event collection",
			proto: &expressionproto.Call{Op: "some", Args: []*expressionproto.Expression{
				protoRef(&expressionproto.Reference{Level: "event"}),
				protoCall(&expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{
					protoRef(&expressionproto.Reference{Name: "name", Level: "event"}),
					protoScalar("exception", ""),
				}}),
			}},
			expected: &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
				&expression.Reference{Level: expression.LevelEvent},
				&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
					&expression.Reference{Level: expression.LevelEvent, Name: expression.EventFieldName},
					&expression.Scalar{Value: "exception"},
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := toFilter(test.proto)
			require.NoError(t, err)
			assert.Equal(t, test.expected, got)
		})
	}
}

func TestToStorageFilter_Rejects(t *testing.T) {
	tests := []struct {
		name        string
		proto       *expressionproto.Call
		expectedErr string
	}{
		{
			name:        "an argument with no term",
			proto:       &expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{protoRef(&expressionproto.Reference{Name: "a"}), {}}},
			expectedErr: "filter argument is empty",
		},
		{
			name: "an argument with no term, nested in a call",
			proto: &expressionproto.Call{Op: "and", Args: []*expressionproto.Expression{
				protoCall(&expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{{}, protoScalar("1", "")}}),
			}},
			expectedErr: "filter argument is empty",
		},
		{
			name:        "an operator this build does not know",
			proto:       &expressionproto.Call{Op: "matches", Args: []*expressionproto.Expression{protoRef(&expressionproto.Reference{Name: "a"}), protoScalar("b", "")}},
			expectedErr: `unknown filter operator "matches"`,
		},
		{
			name:        "a level this build does not know",
			proto:       &expressionproto.Call{Op: "eq", Args: []*expressionproto.Expression{protoRef(&expressionproto.Reference{Name: "a", Level: "pod"}), protoScalar("b", "")}},
			expectedErr: `unknown filter level "pod"`,
		},
		{
			name:        "an empty filter",
			proto:       &expressionproto.Call{},
			expectedErr: `unknown filter operator ""`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := toFilter(test.proto)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}
