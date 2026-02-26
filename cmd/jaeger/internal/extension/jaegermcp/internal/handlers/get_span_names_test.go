// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// mockGetOperationsQueryService mocks the GetOperations method for testing
type mockGetOperationsQueryService struct {
	getOperationsFunc func(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error)
}

func (m *mockGetOperationsQueryService) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	if m.getOperationsFunc != nil {
		return m.getOperationsFunc(ctx, query)
	}
	return nil, nil
}

func TestGetSpanNamesHandler_Handle(t *testing.T) {
	tests := []struct {
		name           string
		input          types.GetSpanNamesInput
		mockOps        []tracestore.Operation
		mockErr        error
		expectedOutput types.GetSpanNamesOutput
		expectedErr    string
	}{
		{
			name: "successful retrieval",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
			},
			mockOps: []tracestore.Operation{
				{Name: "GET /api/users", SpanKind: "SERVER"},
				{Name: "GET /api/orders", SpanKind: "SERVER"},
				{Name: "POST /api/checkout", SpanKind: "SERVER"},
			},
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: []types.SpanNameInfo{
					{Name: "GET /api/orders", SpanKind: "SERVER"},
					{Name: "GET /api/users", SpanKind: "SERVER"},
					{Name: "POST /api/checkout", SpanKind: "SERVER"},
				},
			},
		},
		{
			name: "empty service name",
			input: types.GetSpanNamesInput{
				ServiceName: "",
			},
			expectedErr: "service_name is required",
		},
		{
			name: "with pattern filter",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
				Pattern:     "GET.*",
			},
			mockOps: []tracestore.Operation{
				{Name: "GET /api/users", SpanKind: "SERVER"},
				{Name: "GET /api/orders", SpanKind: "SERVER"},
				{Name: "POST /api/checkout", SpanKind: "SERVER"},
			},
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: []types.SpanNameInfo{
					{Name: "GET /api/orders", SpanKind: "SERVER"},
					{Name: "GET /api/users", SpanKind: "SERVER"},
				},
			},
		},
		{
			name: "with span kind filter",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
				SpanKind:    "CLIENT",
			},
			mockOps: []tracestore.Operation{
				{Name: "call-backend", SpanKind: "CLIENT"},
				{Name: "call-database", SpanKind: "CLIENT"},
			},
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: []types.SpanNameInfo{
					{Name: "call-backend", SpanKind: "CLIENT"},
					{Name: "call-database", SpanKind: "CLIENT"},
				},
			},
		},
		{
			name: "with limit",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
				Limit:       2,
			},
			mockOps: []tracestore.Operation{
				{Name: "op1", SpanKind: "SERVER"},
				{Name: "op2", SpanKind: "SERVER"},
				{Name: "op3", SpanKind: "SERVER"},
				{Name: "op4", SpanKind: "SERVER"},
			},
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: []types.SpanNameInfo{
					{Name: "op1", SpanKind: "SERVER"},
					{Name: "op2", SpanKind: "SERVER"},
				},
			},
		},
		{
			name: "default limit 100",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
			},
			mockOps: generateOperations(150),
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: generateSpanNameInfos(100),
			},
		},
		{
			name: "invalid regex pattern",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
				Pattern:     "[invalid",
			},
			mockOps:     []tracestore.Operation{},
			expectedErr: "invalid pattern",
		},
		{
			name: "storage error",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
			},
			mockErr:     errors.New("storage failure"),
			expectedErr: "failed to get span names",
		},
		{
			name: "no operations found",
			input: types.GetSpanNamesInput{
				ServiceName: "nonexistent",
			},
			mockOps: []tracestore.Operation{},
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: []types.SpanNameInfo{},
			},
		},
		{
			name: "pattern matches nothing",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
				Pattern:     "NOMATCH",
			},
			mockOps: []tracestore.Operation{
				{Name: "GET /api/users", SpanKind: "SERVER"},
			},
			expectedOutput: types.GetSpanNamesOutput{
				SpanNames: []types.SpanNameInfo{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockGetOperationsQueryService{
				getOperationsFunc: func(_ context.Context, _ tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
					return tt.mockOps, tt.mockErr
				},
			}

			handler := &getSpanNamesHandler{queryService: mock}

			_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, tt.input)

			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, output)
		})
	}
}

func TestNewGetSpanNamesHandler(t *testing.T) {
	// Test that NewGetSpanNamesHandler returns a valid handler function
	handler := NewGetSpanNamesHandler(nil)
	assert.NotNil(t, handler)
}

func TestGetSpanNamesOutput_EmptyArrayJSON(t *testing.T) {
	// Test that empty span_names is serialized as [] not null in JSON
	mock := &mockGetOperationsQueryService{
		getOperationsFunc: func(_ context.Context, _ tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
			return []tracestore.Operation{}, nil
		},
	}

	handler := &getSpanNamesHandler{queryService: mock}
	input := types.GetSpanNamesInput{ServiceName: "test-service"}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)
	require.NoError(t, err)

	// Verify the output has an empty slice, not nil
	assert.NotNil(t, output.SpanNames)
	assert.Empty(t, output.SpanNames)
}

// generateOperations creates n operations for testing
func generateOperations(n int) []tracestore.Operation {
	ops := make([]tracestore.Operation, n)
	for i := range n {
		ops[i] = tracestore.Operation{
			Name:     fmt.Sprintf("operation%03d", i),
			SpanKind: "SERVER",
		}
	}
	return ops
}

// generateSpanNameInfos creates n span name infos for testing
func generateSpanNameInfos(n int) []types.SpanNameInfo {
	infos := make([]types.SpanNameInfo, n)
	for i := range n {
		infos[i] = types.SpanNameInfo{
			Name:     fmt.Sprintf("operation%03d", i),
			SpanKind: "SERVER",
		}
	}
	return infos
}
