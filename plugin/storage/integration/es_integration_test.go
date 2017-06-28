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
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/plugin/storage/es/spanstore"
)

func initializeES(s *StorageIntegration) error {
	rand.Seed(time.Now().UnixNano())
	if s.reader != nil || s.writer != nil {
		return nil
	}
	ctx := context.Background()
	rawClient, err := elastic.NewClient()
	if err != nil {
		return err
	}
	client := es.WrapESClient(rawClient)
	logger, _ := testutils.NewLogger()
	writer := spanstore.NewSpanWriter(client, logger)
	reader := spanstore.NewSpanReader(client, logger)

	s.ctx = ctx
	s.cleanUp = eSCleanUp
	s.refresh = eSRefresh
	s.cleanUp()
	s.writer = writer
	s.reader = reader
	s.logger = logger
	return nil
}

func eSCleanUp() error {
	simpleClient, err := elastic.NewSimpleClient()
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
	simpleClient, err := elastic.NewSimpleClient()
	ctx := context.Background()
	if err != nil {
		return err
	}
	simpleClient.Refresh().Do(ctx)
	return nil
}

func TestAll(t *testing.T) {
	if os.Getenv("ESINTEGRATIONTEST") != "" {
		t.Log("Set ESINTEGRATIONTEST env variable to run an integration test on ElasticSearch backend")
		return
	}
	s := &StorageIntegration{}
	require.NoError(t, initializeES(s))
	s.ITestAll(t)
}
