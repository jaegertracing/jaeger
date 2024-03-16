// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"
)

func TestGRPCStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "grpc" {
		t.Skip("Integration test against Jaeger-V2 GRPC skipped; set STORAGE env var to grpc to run this")
	}

	s := &StorageIntegration{
		ConfigFile: "fixtures/grpc_config.yaml",
	}
	s.Test(t)
}
