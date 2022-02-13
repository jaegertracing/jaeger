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
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func TestReflectionServiceValidator(t *testing.T) {
	server := grpc.NewServer()
	reflection.Register(server)

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		err := server.Serve(listener)
		require.NoError(t, err)
	}()
	defer server.Stop()

	ReflectionServiceValidator{
		HostPort:         listener.Addr().String(),
		Server:           server,
		ExpectedServices: []string{"grpc.reflection.v1alpha.ServerReflection"},
	}.Execute(t)
}
