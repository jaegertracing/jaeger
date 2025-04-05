// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func TestServer_GetServices(t *testing.T) {
	tests := []struct {
		name             string
		services         []string
		err              error
		expectedServices []string
		expectedErr      error
	}{
		{
			name:             "success",
			services:         []string{"service1", "service2"},
			expectedServices: []string{"service1", "service2"},
		},
		{
			name:             "empty",
			services:         []string{},
			expectedServices: []string{},
		},
		{
			name:        "error",
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(tracestoremocks.Reader)
			reader.On("GetServices", mock.Anything).
				Return(test.services, test.err).Once()

			server := NewHandler(reader)
			resp, err := server.GetServices(context.Background(), &storage.GetServicesRequest{})
			if test.expectedErr == nil {
				require.Equal(t, test.expectedServices, resp.Services)
			} else {
				require.ErrorIs(t, err, test.expectedErr)
			}
		})
	}
}

func TestServer_GetOperations(t *testing.T) {
	params := tracestore.OperationQueryParams{
		ServiceName: "service",
		SpanKind:    "kind",
	}
	req := &storage.GetOperationsRequest{
		Service:  "service",
		SpanKind: "kind",
	}
	tests := []struct {
		name               string
		operations         []tracestore.Operation
		err                error
		expectedOperations []*storage.Operation
		expectedErr        error
	}{
		{
			name: "success",
			operations: []tracestore.Operation{
				{Name: "operation1", SpanKind: "kind"},
				{Name: "operation2", SpanKind: "kind"},
			},
			expectedOperations: []*storage.Operation{
				{Name: "operation1", SpanKind: "kind"},
				{Name: "operation2", SpanKind: "kind"},
			},
		},
		{
			name:               "empty",
			operations:         []tracestore.Operation{},
			expectedOperations: []*storage.Operation{},
		},
		{
			name:        "error",
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(tracestoremocks.Reader)
			reader.On("GetOperations", mock.Anything, params).
				Return(test.operations, test.err).Once()

			server := NewHandler(reader)
			resp, err := server.GetOperations(context.Background(), req)
			if test.expectedErr == nil {
				require.Equal(t, test.expectedOperations, resp.Operations)
			} else {
				require.ErrorIs(t, err, test.expectedErr)
			}
		})
	}
}
