// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func protoRef(ref *api_v3.Reference) *api_v3.Expression {
	return &api_v3.Expression{Term: &api_v3.Expression_Ref{Ref: ref}}
}

func protoScalar(value string, valueType string) *api_v3.Expression {
	return &api_v3.Expression{Term: &api_v3.Expression_Scalar{Scalar: &api_v3.Scalar{Value: value, Type: valueType}}}
}

func protoCall(call *api_v3.Call) *api_v3.Expression {
	return &api_v3.Expression{Term: &api_v3.Expression_Call{Call: call}}
}

func TestToStorageFilter_NoFilter(t *testing.T) {
	filter, err := toStorageFilter(nil)
	require.NoError(t, err)
	assert.Nil(t, filter)
}

func TestToStorageFilter_Converts(t *testing.T) {
	tests := []struct {
		name     string
		proto    *api_v3.Call
		expected *tracestore.Call
	}{
		{
			name: "an unqualified attribute equality",
			proto: &api_v3.Call{Op: "eq", Args: []*api_v3.Expression{
				protoRef(&api_v3.Reference{Name: "http.status_code"}),
				protoScalar("500", ""),
			}},
			expected: &tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "http.status_code"},
				&tracestore.Scalar{Value: "500"},
			}},
		},
		{
			name: "a level-qualified attribute and a typed constant",
			proto: &api_v3.Call{Op: "gt", Args: []*api_v3.Expression{
				protoRef(&api_v3.Reference{Name: "size", Level: "span", Attr: true}),
				protoScalar("500", "int"),
			}},
			expected: &tracestore.Call{Op: tracestore.OpGt, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "size", Level: tracestore.LevelSpan, Attr: true},
				&tracestore.Scalar{Value: "500", Type: tracestore.ValueTypeInt},
			}},
		},
		{
			name: "a list operand",
			proto: &api_v3.Call{Op: "in", Args: []*api_v3.Expression{
				protoRef(&api_v3.Reference{Name: "http.status_code"}),
				{Term: &api_v3.Expression_List{List: &api_v3.List{Values: []string{"500", "503"}, Type: "int"}}},
			}},
			expected: &tracestore.Call{Op: tracestore.OpIn, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "http.status_code"},
				&tracestore.List{Values: []string{"500", "503"}, Type: tracestore.ValueTypeInt},
			}},
		},
		{
			name: "a nested boolean tree",
			proto: &api_v3.Call{Op: "and", Args: []*api_v3.Expression{
				protoCall(&api_v3.Call{Op: "eq", Args: []*api_v3.Expression{
					protoRef(&api_v3.Reference{Name: "a"}), protoScalar("1", ""),
				}}),
				protoCall(&api_v3.Call{Op: "or", Args: []*api_v3.Expression{
					protoCall(&api_v3.Call{Op: "eq", Args: []*api_v3.Expression{
						protoRef(&api_v3.Reference{Name: "b"}), protoScalar("2", ""),
					}}),
					protoCall(&api_v3.Call{Op: "exists", Args: []*api_v3.Expression{
						protoRef(&api_v3.Reference{Name: "c"}),
					}}),
				}}),
			}},
			expected: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				&tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
					&tracestore.Reference{Name: "a"}, &tracestore.Scalar{Value: "1"},
				}},
				&tracestore.Call{Op: tracestore.OpOr, Args: []tracestore.Expression{
					&tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
						&tracestore.Reference{Name: "b"}, &tracestore.Scalar{Value: "2"},
					}},
					&tracestore.Call{Op: tracestore.OpExists, Args: []tracestore.Expression{
						&tracestore.Reference{Name: "c"},
					}},
				}},
			}},
		},
		{
			name: "a correlated match over the event collection",
			proto: &api_v3.Call{Op: "some", Args: []*api_v3.Expression{
				protoRef(&api_v3.Reference{Level: "event"}),
				protoCall(&api_v3.Call{Op: "eq", Args: []*api_v3.Expression{
					protoRef(&api_v3.Reference{Name: "name", Level: "event"}),
					protoScalar("exception", ""),
				}}),
			}},
			expected: &tracestore.Call{Op: tracestore.OpSome, Args: []tracestore.Expression{
				&tracestore.Reference{Level: tracestore.LevelEvent},
				&tracestore.Call{Op: tracestore.OpEq, Args: []tracestore.Expression{
					tracestore.EventName.Ref(),
					&tracestore.Scalar{Value: "exception"},
				}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := toStorageFilter(test.proto)
			require.NoError(t, err)
			assert.Equal(t, test.expected, got)
		})
	}
}

func TestToStorageFilter_Rejects(t *testing.T) {
	tests := []struct {
		name        string
		proto       *api_v3.Call
		expectedErr string
	}{
		{
			name:        "an argument with no term",
			proto:       &api_v3.Call{Op: "eq", Args: []*api_v3.Expression{protoRef(&api_v3.Reference{Name: "a"}), {}}},
			expectedErr: "filter argument is empty",
		},
		{
			name: "an argument with no term, nested in a call",
			proto: &api_v3.Call{Op: "and", Args: []*api_v3.Expression{
				protoCall(&api_v3.Call{Op: "eq", Args: []*api_v3.Expression{{}, protoScalar("1", "")}}),
			}},
			expectedErr: "filter argument is empty",
		},
		{
			name:        "an operator this build does not know",
			proto:       &api_v3.Call{Op: "matches", Args: []*api_v3.Expression{protoRef(&api_v3.Reference{Name: "a"}), protoScalar("b", "")}},
			expectedErr: `unknown filter operator "matches"`,
		},
		{
			name:        "a level this build does not know",
			proto:       &api_v3.Call{Op: "eq", Args: []*api_v3.Expression{protoRef(&api_v3.Reference{Name: "a", Level: "pod"}), protoScalar("b", "")}},
			expectedErr: `unknown filter level "pod"`,
		},
		{
			name:        "an empty filter",
			proto:       &api_v3.Call{},
			expectedErr: `unknown filter operator ""`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := toStorageFilter(test.proto)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}
