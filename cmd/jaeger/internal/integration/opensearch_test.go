// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestOpenSearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "opensearch")
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-opensearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:  purge,
			Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
		},
	}
	s.e2eInitialize(t, "opensearch")
	s.RunSpanStoreTests(t)
}
