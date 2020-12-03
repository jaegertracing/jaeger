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

package esdependencyreader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

const (
	dependencyType          = "dependencies"
	dependencyIndexBaseName = "jaeger-dependencies"

	timestampField = "timestamp"
)

// DependencyStore defines Elasticsearch dependency store.
type DependencyStore struct {
	client          esclient.ElasticsearchClient
	logger          *zap.Logger
	indexPrefix     string
	indexDateLayout string
	maxDocCount     int
}

var _ dependencystore.Reader = (*DependencyStore)(nil)
var _ dependencystore.Writer = (*DependencyStore)(nil)

// NewDependencyStore creates dependency store.
func NewDependencyStore(client esclient.ElasticsearchClient, logger *zap.Logger, indexPrefix, indexDateLayout string, maxDocCount int) *DependencyStore {
	if indexPrefix != "" {
		indexPrefix += "-"
	}
	return &DependencyStore{
		client:          client,
		logger:          logger,
		indexPrefix:     indexPrefix + dependencyIndexBaseName + "-",
		indexDateLayout: indexDateLayout,
		maxDocCount:     maxDocCount,
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
	return r.client.Index(context.Background(), bytes.NewReader(data), indexWithDate(r.indexPrefix, r.indexDateLayout, ts), dependencyType)
}

// GetDependencies implements dependencystore.Reader
func (r *DependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	searchBody := getSearchBody(endTs, lookback, r.maxDocCount)

	indices := dailyIndices(r.indexPrefix, r.indexDateLayout, endTs, lookback)
	response, err := r.client.Search(ctx, searchBody, r.maxDocCount, indices...)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("%s", response.Error)
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

func getSearchBody(endTs time.Time, lookback time.Duration, maxDocCount int) esclient.SearchBody {
	return esclient.SearchBody{
		Query: &esclient.Query{
			RangeQueries: map[string]esclient.RangeQuery{timestampField: {GTE: endTs.Add(-lookback), LTE: endTs}},
		},
		Size: maxDocCount,
	}
}

func indexWithDate(indexNamePrefix, indexDateLayout string, date time.Time) string {
	return indexNamePrefix + date.UTC().Format(indexDateLayout)
}

func dailyIndices(prefix, format string, ts time.Time, lookback time.Duration) []string {
	var indices []string
	firstIndex := indexWithDate(prefix, format, ts.Add(-lookback))
	currentIndex := indexWithDate(prefix, format, ts)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		ts = ts.Add(-24 * time.Hour)
		currentIndex = indexWithDate(prefix, format, ts)
	}
	return append(indices, firstIndex)
}
