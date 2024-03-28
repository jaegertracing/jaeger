// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
)

func TestBadgerStorage(t *testing.T) {
	s := &StorageIntegration{
		Name: "badger",
		// Since Badger is an embedded local storage, we can't directly test it.
		// That's why we're using a gRPC storage config to connect to the Jaeger remote storage instance,
		// where the Badger storage is running.
		ConfigFile: "fixtures/grpc_config.yaml",
	}
	s.Test(t)
}
