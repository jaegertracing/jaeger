// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

const (
	host          = "0.0.0.0"
	queryPort     = "9200"
	queryHostPort = host + ":" + queryPort
	queryURL      = "http://" + queryHostPort
)

type ESStorageIntegration struct {
	E2EStorageIntegration
	esClient *integration.EsClient
}

func (s *ESStorageIntegration) initializeES(t *testing.T) {
	s.esClient = integration.StartEsClient(t, queryURL)
	s.CleanUp = cleanUp
}

func TestESStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")

	s := &ESStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "../../config-elasticsearch.yaml",
			StorageIntegration: integration.StorageIntegration{
				Fixtures:                     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
				SkipBinaryAttrs:              true,
				GetOperationsMissingSpanKind: true,
			},
		},
	}
	s.initializeES(t)
	s.e2eInitialize(t, "elasticsearch")
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}

func TestOSStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "opensearch")

	s := &ESStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "../../config-opensearch.yaml",
			StorageIntegration: integration.StorageIntegration{
				Fixtures:                     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
				SkipBinaryAttrs:              true,
				GetOperationsMissingSpanKind: true,
			},
		},
	}
	s.initializeES(t)
	s.e2eInitialize(t, "opensearch")
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}
