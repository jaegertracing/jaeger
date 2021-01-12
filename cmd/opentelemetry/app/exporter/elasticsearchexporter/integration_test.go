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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/reader/es/esdependencyreader"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/reader/es/esspanreader"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

const (
	host               = "0.0.0.0"
	esPort             = "9200"
	esHostPort         = host + ":" + esPort
	esURL              = "http://" + esHostPort
	indexPrefix        = "integration-test"
	indexDateLayout    = "2006-01-02"
	tagKeyDeDotChar    = "@"
	maxSpanAge         = time.Hour * 72
	numShards          = 5
	numReplicas        = 0
	defaultMaxDocCount = 10_000
)

type IntegrationTest struct {
	integration.StorageIntegration

	logger *zap.Logger
}

func (s *IntegrationTest) initializeES(allTagsAsFields bool) error {
	s.logger, _ = testutils.NewLogger()

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
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/*", esURL), strings.NewReader(""))
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	err = response.Body.Close()
	if err != nil {
		return err
	}
	// initialize writer, it caches service names
	return s.initSpanstore(allTagsAsFields)
}

func (s *IntegrationTest) initSpanstore(allTagsAsFields bool) error {
	cfg := config.Configuration{
		Servers:         []string{esURL},
		IndexPrefix:     indexPrefix,
		IndexDateLayout: indexDateLayout,
		Tags: config.TagsAsFields{
			AllAsFields: allTagsAsFields,
		},
	}
	w, err := newEsSpanWriter(cfg, s.logger, false, "")
	if err != nil {
		return err
	}
	esVersion := uint(w.esClientVersion())
	spanMapping, serviceMapping := es.GetSpanServiceMappings(numShards, numReplicas, esVersion)
	err = w.CreateTemplates(context.Background(), spanMapping, serviceMapping)
	if err != nil {
		return err
	}
	s.SpanWriter = singleSpanWriter{
		writer:    w,
		converter: dbmodel.NewFromDomain(allTagsAsFields, []string{}, tagKeyDeDotChar),
	}

	elasticsearchClient, err := esclient.NewElasticsearchClient(cfg, s.logger)
	if err != nil {
		return err
	}
	reader := esspanreader.NewEsSpanReader(elasticsearchClient, s.logger, esspanreader.Config{
		IndexPrefix:       indexPrefix,
		IndexDateLayout:   indexDateLayout,
		TagDotReplacement: tagKeyDeDotChar,
		MaxSpanAge:        maxSpanAge,
		MaxDocCount:       defaultMaxDocCount,
	})
	s.SpanReader = reader

	depMapping := es.GetDependenciesMappings(numShards, numReplicas, esVersion)
	depStore := esdependencyreader.NewDependencyStore(elasticsearchClient, s.logger, indexPrefix, indexDateLayout, defaultMaxDocCount)
	if err := depStore.CreateTemplates(depMapping); err != nil {
		return nil
	}
	s.DependencyReader = depStore
	s.DependencyWriter = depStore
	return nil
}

func (s *IntegrationTest) esRefresh() error {
	response, err := http.Post(fmt.Sprintf("%s/_refresh", esURL), "application/json", strings.NewReader(""))
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func healthCheck() error {
	for i := 0; i < 200; i++ {
		if _, err := http.Get(esURL); err == nil {
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
