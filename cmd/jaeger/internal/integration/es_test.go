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
	s.CleanUp = func(t *testing.T) {
		s.esClient.EsClientCleanup(t)
	}
	s.Refresh = s.esClient.EsClientRefresh
	s.esClient.EsClientCleanup(t)
	// TODO: remove this flag after ES support returning spanKind when get operations
	s.GetOperationsMissingSpanKind = true
}

func TestESStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch", "opensearch")

	s := &ESStorageIntegration{}
	s.initializeES(t)
	s.Fixtures = integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json")
	s.ConfigFile = "cmd/jaeger/config-elasticsearch.yaml"
	s.SkipBinaryAttrs = true
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunSpanStoreTests(t)
}
