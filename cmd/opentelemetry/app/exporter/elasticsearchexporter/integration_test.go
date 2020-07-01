// Copyright (c) 2019 The Jaeger Authors.
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

// +build integration

package elasticsearchexporter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	eswrapper "github.com/jaegertracing/jaeger/pkg/es/wrapper"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

const (
	host            = "0.0.0.0"
	queryPort       = "9200"
	queryHostPort   = host + ":" + queryPort
	queryURL        = "http://" + queryHostPort
	indexPrefix     = "integration-test"
	tagKeyDeDotChar = "@"
	maxSpanAge      = time.Hour * 72
)

type IntegrationTest struct {
	integration.StorageIntegration

	client        *elastic.Client
	bulkProcessor *elastic.BulkProcessor
	logger        *zap.Logger
}

type storageWrapper struct {
	writer *esSpanWriter
}

func (s storageWrapper) WriteSpan(span *model.Span) error {
	// This fails because there is no binary tag type in OTEL and also OTEL span's status code is always created
	//traces := jaegertranslator.ProtoBatchesToInternalTraces([]*model.Batch{{Process: span.Process, Spans: []*model.Span{span}}})
	//_, err := s.writer.WriteTraces(context.Background(), traces)
	converter := dbmodel.FromDomain{}
	dbSpan := converter.FromDomainEmbedProcess(span)
	_, err := s.writer.writeSpans([]*dbmodel.Span{dbSpan})
	return err
}

func (s *IntegrationTest) getVersion() (uint, error) {
	pingResult, _, err := s.client.Ping(queryURL).Do(context.Background())
	if err != nil {
		return 0, err
	}
	esVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
	if err != nil {
		return 0, err
	}
	return uint(esVersion), nil
}

func (s *IntegrationTest) initializeES(allTagsAsFields bool) error {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false))
	if err != nil {
		return err
	}
	s.logger, _ = testutils.NewLogger()

	s.client = rawClient
	s.initSpanstore(allTagsAsFields)
	s.CleanUp = func() error {
		return s.esCleanUp(allTagsAsFields)
	}
	s.Refresh = s.esRefresh
	s.esCleanUp(allTagsAsFields)
	// TODO: remove this flag after ES support returning spanKind when get operations
	s.NotSupportSpanKindWithOperation = true
	return nil
}

func (s *IntegrationTest) esCleanUp(allTagsAsFields bool) error {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	if err != nil {
		return err
	}
	return s.initSpanstore(allTagsAsFields)
}

func (s *IntegrationTest) initSpanstore(allTagsAsFields bool) error {
	bp, _ := s.client.BulkProcessor().BulkActions(1).FlushInterval(time.Nanosecond).Do(context.Background())
	s.bulkProcessor = bp
	esVersion, err := s.getVersion()
	if err != nil {
		return err
	}
	client := eswrapper.WrapESClient(s.client, bp, esVersion)
	spanMapping, serviceMapping := es.GetSpanServiceMappings(5, 1, client.GetVersion())

	w, err := newEsSpanWriter(config.Configuration{
		Servers:     []string{queryURL},
		IndexPrefix: indexPrefix,
		Tags: config.TagsAsFields{
			AllAsFields: allTagsAsFields,
		},
	}, s.logger)
	if err != nil {
		return err
	}
	err = w.CreateTemplates(spanMapping, serviceMapping)
	if err != nil {
		return err
	}
	s.SpanWriter = storageWrapper{
		writer: w,
	}
	s.SpanReader = spanstore.NewSpanReader(spanstore.SpanReaderParams{
		Client:            client,
		Logger:            s.logger,
		MetricsFactory:    metrics.NullFactory,
		IndexPrefix:       indexPrefix,
		MaxSpanAge:        maxSpanAge,
		TagDotReplacement: tagKeyDeDotChar,
	})
	dependencyStore := dependencystore.NewDependencyStore(client, s.logger, indexPrefix)
	depMapping := es.GetDependenciesMappings(5, 1, client.GetVersion())
	err = dependencyStore.CreateTemplates(depMapping)
	if err != nil {
		return err
	}
	s.DependencyReader = dependencyStore
	s.DependencyWriter = dependencyStore
	return nil
}

func (s *IntegrationTest) esRefresh() error {
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

func testElasticsearchStorage(t *testing.T, allTagsAsFields bool) {
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &IntegrationTest{
		StorageIntegration: integration.StorageIntegration{
			FixturesPath: "../../../../../plugin/storage/integration",
		},
	}
	require.NoError(t, s.initializeES(allTagsAsFields))
	s.Fixtures = integration.LoadAndParseQueryTestCases(t, "../../../../../plugin/storage/integration/fixtures/queries_es.json")
	s.IntegrationTestAll(t)
}

func TestElasticsearchStorage(t *testing.T) {
	testElasticsearchStorage(t, false)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	testElasticsearchStorage(t, true)
}
