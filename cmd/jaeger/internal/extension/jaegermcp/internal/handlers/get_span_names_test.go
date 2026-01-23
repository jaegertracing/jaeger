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
		name          string
		input         types.GetSpanNamesInput
		mockOps       []tracestore.Operation
		mockErr       error
		expectedCount int
		expectedErr   string
		expectedNames []string
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
			expectedCount: 3,
			expectedNames: []string{"GET /api/orders", "GET /api/users", "POST /api/checkout"},
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
			expectedCount: 2,
			expectedNames: []string{"GET /api/orders", "GET /api/users"},
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
			expectedCount: 2,
			expectedNames: []string{"call-backend", "call-database"},
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
			expectedCount: 2,
			expectedNames: []string{"op1", "op2"},
		},
		{
			name: "default limit 100",
			input: types.GetSpanNamesInput{
				ServiceName: "frontend",
			},
			mockOps:       generateOperations(150),
			expectedCount: 100,
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
			mockOps:       []tracestore.Operation{},
			expectedCount: 0,
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
			expectedCount: 0,
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
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, output.SpanNames, tt.expectedCount)

			if len(tt.expectedNames) > 0 {
				actualNames := make([]string, len(output.SpanNames))
				for i, sn := range output.SpanNames {
					actualNames[i] = sn.Name
				}
				assert.Equal(t, tt.expectedNames, actualNames)
			}
		})
	}
}

func TestNewGetSpanNamesHandler(t *testing.T) {
	mock := &mockGetOperationsQueryService{}
	handler := &getSpanNamesHandler{queryService: mock}
	assert.NotNil(t, handler)
}

// generateOperations creates n operations for testing
func generateOperations(n int) []tracestore.Operation {
	ops := make([]tracestore.Operation, n)
	for i := 0; i < n; i++ {
		ops[i] = tracestore.Operation{
			Name:     fmt.Sprintf("operation%03d", i),
			SpanKind: "SERVER",
		}
	}
	return ops
}
