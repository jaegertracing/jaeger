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

package dependencystore

import (
	"context"
	"encoding/json"
	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es"
	"go.uber.org/zap"
	"time"
)

type timeToDependencies struct {
	Ts           uint64                 `json:"ts"`
	Dependencies []model.DependencyLink `json:"dependencies"`
}

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	ctx    context.Context
	client es.Client
	logger *zap.Logger
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(client es.Client, logger *zap.Logger) *DependencyStore {
	ctx := context.Background()
	return &DependencyStore{
		ctx:    ctx,
		client: client,
		logger: logger,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	indexName := indexName(ts)

	s.client.CreateIndex(indexName).Body(dependenciesMapping).Do(s.ctx)
	s.client.Index().Index(indexName).
		Type("dependencies").
		BodyJson(&timeToDependencies{model.TimeAsEpochMicroseconds(ts), dependencies}).
		Do(s.ctx)

	return nil
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	searchResult, err := s.client.Search(getIndices(endTs, lookback)...).
		Type("dependencies").
		Size(10000).
		Query(buildTSQuery(endTs, lookback)).
		Do(s.ctx)
	if err != nil {
		return nil, err
	}

	var retDependencies []model.DependencyLink
	hits := searchResult.Hits.Hits
	if len(hits) == 0 {
		return nil, errors.New("wtf man")
	}
	for _, hit := range hits {
		source := hit.Source
		var timeToDependencies timeToDependencies
		if err := json.Unmarshal(*source, &timeToDependencies); err != nil {
			return nil, err
		}
		retDependencies = append(retDependencies, timeToDependencies.Dependencies...)
	}
	return retDependencies, nil
}

func buildTSQuery(endTs time.Time, lookback time.Duration) elastic.Query {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(endTs.Add(-lookback))
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(endTs)
	return elastic.NewRangeQuery("ts").Gte(minStartTimeMicros).Lte(maxStartTimeMicros)
}

func getIndices(ts time.Time, lookback time.Duration) []string {
	var indices []string
	for int(lookback) > 0 {
		indices = append(indices, indexName(ts))
		ts = ts.Add(-24 * time.Hour)
		lookback -= 24 * time.Hour
	}
	return indices
}

func indexName(date time.Time) string {
	return "jaeger-dependencies-" + date.Format("2006-01-02")
}
