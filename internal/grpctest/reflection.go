// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpctest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_reflection "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// ReflectionServiceValidator verifies that a gRPC service at a given address
// supports reflection service. Called must invoke Execute func.
type ReflectionServiceValidator struct {
	HostPort         string
	ExpectedServices []string
}

// Execute performs validation.
func (v ReflectionServiceValidator) Execute(t *testing.T) {
	t.Helper()
	conn, err := grpc.NewClient(
		v.HostPort,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := grpc_reflection.NewServerReflectionClient(conn)
	r, err := client.ServerReflectionInfo(context.Background())
	require.NoError(t, err)
	require.NotNil(t, r)

	err = r.Send(&grpc_reflection.ServerReflectionRequest{
		MessageRequest: &grpc_reflection.ServerReflectionRequest_ListServices{},
	})
	require.NoError(t, err)
	m, err := r.Recv()
	require.NoError(t, err)
	require.IsType(t,
		new(grpc_reflection.ServerReflectionResponse_ListServicesResponse),
		m.MessageResponse)

	resp := m.MessageResponse.(*grpc_reflection.ServerReflectionResponse_ListServicesResponse)
	for _, svc := range v.ExpectedServices {
		var found string
		for _, s := range resp.ListServicesResponse.Service {
			if svc == s.Name {
				found = s.Name
				break
			}
		}
		require.Equalf(t, svc, found,
			"service not found, got '%+v'",
			resp.ListServicesResponse.Service)
	}
}
