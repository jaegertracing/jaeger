// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpctest

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestReflectionServiceValidator(t *testing.T) {
	server := grpc.NewServer()
	reflection.Register(server)

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		err := server.Serve(listener)
		assert.NoError(t, err)
	}()
	defer server.Stop()

	ReflectionServiceValidator{
		HostPort:         listener.Addr().String(),
		ExpectedServices: []string{"grpc.reflection.v1alpha.ServerReflection"},
	}.Execute(t)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
