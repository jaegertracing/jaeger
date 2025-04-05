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
