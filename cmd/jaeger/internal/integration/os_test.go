// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestOSStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "opensearch")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-opensearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:                      purge,
			Fixtures:                     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			SkipBinaryAttrs:              true,
			GetOperationsMissingSpanKind: true,
		},
	}
	s.e2eInitialize(t, "opensearch")
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}
