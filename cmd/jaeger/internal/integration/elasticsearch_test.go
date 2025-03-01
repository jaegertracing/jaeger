// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

func TestElasticsearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:  purge,
			Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	s.RunSpanStoreTests(t)
}
