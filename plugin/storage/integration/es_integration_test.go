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
	"os"
	"testing"
	"time"
	"net/http"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/require"
	"github.com/pkg/errors"

	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/plugin/storage/es/spanstore"
)

const (
	host          = "0.0.0.0"
	queryPort     = "9200"
	queryHostPort = host + ":" + queryPort
	queryURL      = "http://" + queryHostPort
	username = "elastic" // the elasticsearch default username
	password = "changeme" // the elasticsearch default password
)

func initializeES(s *StorageIntegration) error {
	if s.reader != nil || s.writer != nil {
		return nil
	}
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetBasicAuth(username, password),
		elastic.SetSniff(false))
	if err != nil {
		return err
	}
	client := es.WrapESClient(rawClient)
	logger, _ := testutils.NewLogger()
	writer := spanstore.NewSpanWriter(client, logger)
	reader := spanstore.NewSpanReader(client, logger)

	s.cleanUp = eSCleanUp
	s.refresh = eSRefresh
	s.cleanUp()
	s.writer = writer
	s.reader = reader
	s.logger = logger
	return nil
}

func eSCleanUp() error {
	simpleClient, err := elastic.NewSimpleClient(
		elastic.SetURL(queryURL),
		elastic.SetBasicAuth(username, password),
		elastic.SetSniff(false))
	ctx := context.Background()
	if err != nil {
		return err
	}
	simpleClient.DeleteIndex(spanstore.IndexWithDate(time.Now())).Do(ctx)
	simpleClient.DeleteIndex(spanstore.IndexWithDate(time.Now().AddDate(0, 0, -1))).Do(ctx)
	simpleClient.DeleteIndex(spanstore.IndexWithDate(time.Now().AddDate(0, 0, -2))).Do(ctx)
	return nil
}

func eSRefresh() error {
	simpleClient, err := elastic.NewSimpleClient(
		elastic.SetURL(queryURL),
		elastic.SetBasicAuth(username, password),
		elastic.SetSniff(false))
	ctx := context.Background()
	if err != nil {
		return err
	}
	simpleClient.Refresh().Do(ctx)
	return nil
}

func healthCheck() error {
	for i := 0; i < 100; i++ {
		if _, err := http.Get(queryURL); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("query service is not ready")
}

// DO NOT RUN IF YOU HAVE IMPORTANT SPANS IN ELASTICSEARCH
func TestAll(t *testing.T) {
	if os.Getenv("ESINTEGRATIONTEST") == "" {
		t.Log("Set ESINTEGRATIONTEST env variable to run an integration test on ElasticSearch backend")
		return
	}
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &StorageIntegration{}
	require.NoError(t, initializeES(s))
	s.IntegrationTestAll(t)
}
