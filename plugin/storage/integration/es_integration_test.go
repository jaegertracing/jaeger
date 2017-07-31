// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package integration

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/plugin/storage/es/dependencystore"
	"github.com/uber/jaeger/plugin/storage/es/spanstore"
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
	s.spanWriter = spanstore.NewSpanWriter(client, s.logger, metrics.NullFactory)
	s.spanReader = spanstore.NewSpanReader(client, s.logger, 72*time.Hour, 24*time.Hour, metrics.NullFactory)
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
	if os.Getenv("ES_INTEGRATION_TEST") == "" {
		t.Skip("Set ES_INTEGRATION_TEST env variable to run an integration test on ElasticSearch backend")
	}
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &ESStorageIntegration{}
	require.NoError(t, s.initializeES())
	s.IntegrationTestAll(t)
}
