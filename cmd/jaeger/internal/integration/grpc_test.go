// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

type GRPCStorageIntegration struct {
	E2EStorageIntegration

	server *integration.GRPCServer
}

func (s *GRPCStorageIntegration) initialize(t *testing.T) {
	var err error
	s.server, err = integration.NewGRPCServer()
	require.NoError(t, err)
	require.NoError(t, s.server.Start())

	s.Refresh = func(_ *testing.T) {}
	s.CleanUp = s.cleanUp
}

func (s *GRPCStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.server.Close())
	s.initialize(t)
}

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	server, err := integration.NewGRPCServer()
	require.NoError(t, err)
	s := &GRPCStorageIntegration{
		server: server,
	}
	s.ConfigFile = "cmd/jaeger/grpc_config.yaml"
	s.initialize(t)
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}
