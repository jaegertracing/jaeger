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
)

type ESStorageIntegration struct {
	client *elastic.Client
	StorageIntegration
}

func (s *ESStorageIntegration) initializeES() error {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetBasicAuth(username, password),
		elastic.SetSniff(false))
	if err != nil {
		return err
	}
	logger, _ := testutils.NewLogger()

	s.client = rawClient
	s.logger = logger

	client := es.WrapESClient(s.client)
	dependencyStore := dependencystore.NewDependencyStore(client, logger)
	s.dependencyReader = dependencyStore
	s.dependencyWriter = dependencyStore
	s.initSpanstore()
	s.cleanUp = s.esCleanUp
	s.refresh = s.esRefresh
	s.cleanUp()
	return nil
}

func (s *ESStorageIntegration) esCleanUp() error {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	s.initSpanstore()
	return err
}

func (s *ESStorageIntegration) initSpanstore() {
	client := es.WrapESClient(s.client)
	s.spanWriter = spanstore.NewSpanWriter(client, s.logger, metrics.NullFactory, 0, 0)
	s.spanReader = spanstore.NewSpanReader(client, s.logger, 72*time.Hour, metrics.NullFactory)
}

func (s *ESStorageIntegration) esRefresh() error {
	_, err := s.client.Refresh().Do(context.Background())
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

func TestAll(t *testing.T) {
	if os.Getenv("STORAGE") != "es" {
		t.Skip("Integration test against ElasticSearch skipped; set STORAGE env var to es to run this")
	}
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &ESStorageIntegration{}
	require.NoError(t, s.initializeES())
	s.IntegrationTestAll(t)
}
