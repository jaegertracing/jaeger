// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	s := &E2EStorageIntegration{
		ConfigFile:         "../../config-remote-storage.yaml",
		SkipStorageCleaner: true,
	}
	s.e2eInitialize(t, "grpc")
	s.RunSpanStoreTests(t)
}
