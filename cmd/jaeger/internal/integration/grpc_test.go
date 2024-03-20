// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
)

func TestGRPCStorage(t *testing.T) {
	s := &StorageIntegration{
		Name:       "grpc",
		ConfigFile: "fixtures/grpc_config.yaml",
	}
	s.Test(t)
}
