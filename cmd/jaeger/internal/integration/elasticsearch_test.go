// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestElasticsearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:                      purge,
			Fixtures:                     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			GetOperationsMissingSpanKind: true,
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	s.RunSpanStoreTests(t)
}

func TestElasticsearchRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")
	e := &E2EElasticSearchILMIntegration{
		isOpenSearch: false,
	}
	e.RunTests(t)
}
