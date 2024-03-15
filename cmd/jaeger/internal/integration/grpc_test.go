// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"
)

func TestGRPCStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "jaeger_grpc" {
		t.Skip("Integration test against Jaeger-V2 GRPC skipped; set STORAGE env var to jaeger_grpc to run this")
	}

	s := &StorageIntegration{
		ConfigFile: "fixtures/grpc_config.yaml",
	}
	s.Test(t)
}
