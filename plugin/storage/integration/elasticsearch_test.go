// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
)

const (
	host          = "0.0.0.0"
	queryPort     = "9200"
	queryHostPort = host + ":" + queryPort
	queryURL      = "http://" + queryHostPort
	username      = "elastic"  // the elasticsearch default username
	password      = "changeme" // the elasticsearch default password
	indexPrefix = "integration-test"
)

type ESStorageIntegration struct {
	StorageIntegration

	client        *elastic.Client
	bulkProcessor *elastic.BulkProcessor
	logger        *zap.Logger
}

func (s *ESStorageIntegration) initializeES() error {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetBasicAuth(username, password),
		elastic.SetSniff(false))
	if err != nil {
		return err
	}
	s.logger, _ = testutils.NewLogger()

	s.client = rawClient

	s.bulkProcessor, _ = s.client.BulkProcessor().Do(context.Background())
	client := es.WrapESClient(s.client, s.bulkProcessor)
	dependencyStore := dependencystore.NewDependencyStore(client, s.logger, indexPrefix)
	s.DependencyReader = dependencyStore
	s.DependencyWriter = dependencyStore
	s.initSpanstore()
	s.CleanUp = s.esCleanUp
	s.Refresh = s.esRefresh
	s.esCleanUp()
	return nil
}

func (s *ESStorageIntegration) esCleanUp() error {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	s.initSpanstore()
	return err
}

func (s *ESStorageIntegration) initSpanstore() {
	bp, _ := s.client.BulkProcessor().BulkActions(1).FlushInterval(time.Nanosecond).Do(context.Background())
	client := es.WrapESClient(s.client, bp)
	s.SpanWriter = spanstore.NewSpanWriter(client, s.logger, metrics.NullFactory, 0, 0, indexPrefix)
	s.SpanReader = spanstore.NewSpanReader(client, s.logger, 72*time.Hour, metrics.NullFactory, indexPrefix)
}

func (s *ESStorageIntegration) esRefresh() error {
	err := s.bulkProcessor.Flush()
	if err != nil {
		return err
	}
	_, err = s.client.Refresh().Do(context.Background())
	return err
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

func TestElasticsearchStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "elasticsearch" {
		t.Skip("Integration test against ElasticSearch skipped; set STORAGE env var to elasticsearch to run this")
	}
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &ESStorageIntegration{}
	require.NoError(t, s.initializeES())
	s.IntegrationTestAll(t)
}
