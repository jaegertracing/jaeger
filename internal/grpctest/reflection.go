// Copyright (c) 2022 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpctest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// ReflectionServiceValidator verifies that a gRPC service at a given address
// supports reflection service. Called must invoke Execute func.
type ReflectionServiceValidator struct {
	Server           *grpc.Server
	HostPort         string
	ExpectedServices []string
}

// Execute performs validation.
func (v ReflectionServiceValidator) Execute(t *testing.T) {
	conn, err := grpc.Dial(
		v.HostPort,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
	r, err := client.ServerReflectionInfo(context.Background())
	require.NoError(t, err)
	require.NotNil(t, r)

	err = r.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{},
	})
	require.NoError(t, err)
	m, err := r.Recv()
	require.NoError(t, err)
	require.IsType(t,
		new(grpc_reflection_v1alpha.ServerReflectionResponse_ListServicesResponse),
		m.MessageResponse)

	resp := m.MessageResponse.(*grpc_reflection_v1alpha.ServerReflectionResponse_ListServicesResponse)
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
