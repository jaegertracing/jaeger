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
	"go.uber.org/zap"
	"time"
	"github.com/uber/jaeger/pkg/es"
	"context"
	"github.com/uber/jaeger/model"
)

// DependencyStore handles all queries and insertions to ElasticSearch dependencies
type DependencyStore struct {
	ctx    context.Context
	client es.Client
	dependencyDataFrequency time.Duration
	logger                  *zap.Logger
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(client es.Client, dependencyDataFrequency time.Duration, logger *zap.Logger) *DependencyStore {
	ctx := context.Background()
	return &DependencyStore{
		ctx: ctx,
		client: client,
		dependencyDataFrequency: dependencyDataFrequency,
		logger:                  logger,
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	return nil
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return nil
}