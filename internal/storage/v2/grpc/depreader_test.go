// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
)

// testDependenciesServer implements the storage.DependencyReaderServer interface
// to simulate responses for testing.
type testDependenciesServer struct {
	storage.UnimplementedDependencyReaderServer

	dependencies []*storage.Dependency
	err          error
}

func (t *testDependenciesServer) GetDependencies(
	context.Context,
	*storage.GetDependenciesRequest,
) (*storage.GetDependenciesResponse, error) {
	return &storage.GetDependenciesResponse{
		Dependencies: t.dependencies,
	}, t.err
}

func startTestDependenciesServer(t *testing.T, testServer *testDependenciesServer) *grpc.ClientConn {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := grpc.NewServer()
	storage.RegisterDependencyReaderServer(server, testServer)

	return startServer(t, server, listener)
}

func TestDependencyReader_GetDependencies(t *testing.T) {
	tests := []struct {
		name                 string
		testServer           *testDependenciesServer
		expectedDependencies []model.DependencyLink
		expectedError        string
	}{
		{
			name: "success",
			testServer: &testDependenciesServer{
				dependencies: []*storage.Dependency{
					{
						Parent:    "service-a",
						Child:     "service-b",
						CallCount: 42,
						Source:    "source",
					},
					{
						Parent:    "service-c",
						Child:     "service-d",
						CallCount: 24,
						Source:    "source",
					},
				},
			},
			expectedDependencies: []model.DependencyLink{
				{
					Parent:    "service-a",
					Child:     "service-b",
					CallCount: 42,
					Source:    "source",
				},
				{
					Parent:    "service-c",
					Child:     "service-d",
					CallCount: 24,
					Source:    "source",
				},
			},
		},
		{
			name: "empty",
			testServer: &testDependenciesServer{
				dependencies: []*storage.Dependency{},
			},
			expectedDependencies: []model.DependencyLink{},
		},
		{
			name: "error",
			testServer: &testDependenciesServer{
				err: assert.AnError,
			},
			expectedError: "failed to get dependencies",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := startTestDependenciesServer(t, test.testServer)

			reader := NewDependencyReader(conn)
			dependencies, err := reader.GetDependencies(context.Background(), depstore.QueryParameters{})

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
			} else {
				require.Equal(t, test.expectedDependencies, dependencies)
			}
		})
	}
}
