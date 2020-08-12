// Copyright (c) 2020 The Jaeger Authors.
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

package dependencystore

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esclient"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

const (
	dependencyType          = "dependencies"
	dependencyIndexBaseName = "jaeger-dependencies"

	timestampField = "timestamp"

	// default number of documents to fetch in a query
	// see search.max_buckets and index.max_result_window
	defaultDocCount = 10_000
	indexDateFormat = "2006-01-02" // date format for index e.g. 2020-01-20
)

// DependencyStore defines Elasticsearch dependency store.
type DependencyStore struct {
	client      esclient.ElasticsearchClient
	logger      *zap.Logger
	indexPrefix string
}

var _ dependencystore.Reader = (*DependencyStore)(nil)
var _ dependencystore.Writer = (*DependencyStore)(nil)

// NewDependencyStore creates dependency store.
func NewDependencyStore(client esclient.ElasticsearchClient, logger *zap.Logger, indexPrefix string) *DependencyStore {
	if indexPrefix != "" {
		indexPrefix += "-"
	}
	return &DependencyStore{
		client:      client,
		logger:      logger,
		indexPrefix: indexPrefix + dependencyIndexBaseName + "-",
	}
}

// CreateTemplates creates index templates for dependency index
func (r *DependencyStore) CreateTemplates(dependenciesTemplate string) error {
	return r.client.PutTemplate(context.Background(), dependencyIndexBaseName, strings.NewReader(dependenciesTemplate))
}

// WriteDependencies implements dependencystore.Writer
func (r *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	d := &dbmodel.TimeDependencies{
		Timestamp:    ts,
		Dependencies: dbmodel.FromDomainDependencies(dependencies),
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return r.client.Index(context.Background(), bytes.NewReader(data), indexWithDate(r.indexPrefix, ts), dependencyType)
}

// GetDependencies implements dependencystore.Reader
func (r *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	searchBody := getSearchBody(endTs, lookback)

	indices := dailyIndices(r.indexPrefix, endTs, lookback)
	response, err := r.client.Search(context.Background(), searchBody, defaultDocCount, indices...)
	if err != nil {
		return nil, err
	}

	var dependencies []dbmodel.DependencyLink
	for _, hit := range response.Hits.Hits {
		var d dbmodel.TimeDependencies
		if err := json.Unmarshal(*hit.Source, &d); err != nil {
			return nil, err
		}
		dependencies = append(dependencies, d.Dependencies...)
	}
	return dbmodel.ToDomainDependencies(dependencies), nil
}

func getSearchBody(endTs time.Time, lookback time.Duration) esclient.SearchBody {
	return esclient.SearchBody{
		Query: &esclient.Query{
			RangeQueries: map[string]esclient.RangeQuery{timestampField: {GTE: endTs.Add(-lookback), LTE: endTs}},
		},
		Size: defaultDocCount,
	}
}

func indexWithDate(indexNamePrefix string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format(indexDateFormat)
}

func dailyIndices(prefix string, ts time.Time, lookback time.Duration) []string {
	var indices []string
	firstIndex := indexWithDate(prefix, ts.Add(-lookback))
	currentIndex := indexWithDate(prefix, ts)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		ts = ts.Add(-24 * time.Hour)
		currentIndex = indexWithDate(prefix, ts)
	}
	return append(indices, firstIndex)
}
