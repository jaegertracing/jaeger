// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

type ESStorageIntegration struct {
	E2EStorageIntegration

	client   *elastic.Client
	v8Client *elasticsearch8.Client
	logger   *zap.Logger
}

const (
	host                     = "0.0.0.0"
	queryPort                = "9200"
	queryHostPort            = host + ":" + queryPort
	queryURL                 = "http://" + queryHostPort
	indexPrefix              = "integration-test"
	indexDateLayout          = "2006-01-02"
	tagKeyDeDotChar          = "@"
	maxSpanAge               = time.Hour * 72
	defaultMaxDocCount       = 10_000
	spanTemplateName         = "jaeger-span"
	serviceTemplateName      = "jaeger-service"
	dependenciesTemplateName = "jaeger-dependencies"
	primaryNamespace         = "es"
	archiveNamespace         = "es-archive"
)

func (s *ESStorageIntegration) initializeES(t *testing.T) {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false))
	require.NoError(t, err)
	s.logger, _ = testutils.NewLogger()

	s.client = rawClient
	s.v8Client, err = elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
	})
	require.NoError(t, err)

	s.CleanUp = func(t *testing.T) {
		s.esCleanUp(t)
	}
	s.Refresh = s.esRefresh
	s.esCleanUp(t)
	// TODO: remove this flag after ES support returning spanKind when get operations
	s.GetOperationsMissingSpanKind = true
}

func (s *ESStorageIntegration) esCleanUp(t *testing.T) {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
}

func (s *ESStorageIntegration) esRefresh(t *testing.T) {
	_, err := s.client.Refresh().Do(context.Background())
	require.NoError(t, err)
}

func healthCheck() error {
	for i := 0; i < 200; i++ {
		if _, err := http.Get(queryURL); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("elastic search is not ready")
}

func TestESStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch", "opensearch")

	require.NoError(t, healthCheck())
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
