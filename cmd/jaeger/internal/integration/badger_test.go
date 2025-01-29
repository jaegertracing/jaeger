// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-badger.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp: purge,

			// TODO: remove this once badger supports returning spanKind from GetOperations
			// Cf https://github.com/jaegertracing/jaeger/issues/1922
			GetOperationsMissingSpanKind: true,
		},
	}
	s.e2eInitialize(t, "badger")
	s.RunAll(t)
}
