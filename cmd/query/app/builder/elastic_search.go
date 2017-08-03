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

package builder

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/es"
	escfg "github.com/uber/jaeger/pkg/es/config"
	esDependencyStore "github.com/uber/jaeger/plugin/storage/es/dependencystore"
	esSpanstore "github.com/uber/jaeger/plugin/storage/es/spanstore"
	"github.com/uber/jaeger/storage/dependencystore"
	"github.com/uber/jaeger/storage/spanstore"
	"time"
)

type esBuilder struct {
	logger         *zap.Logger
	metricsFactory metrics.Factory
	client         es.Client
	maxLookback    time.Duration
}

func newESBuilder(cBuilder escfg.ClientBuilder, logger *zap.Logger, metricsFactory metrics.Factory, maxLookBack time.Duration) (*esBuilder, error) {
	client, err := cBuilder.NewClient()
	if err != nil {
		return nil, err
	}

	return &esBuilder{
		client:         client,
		logger:         logger,
		metricsFactory: metricsFactory,
		maxLookback:    maxLookBack,
	}, nil
}

func (e *esBuilder) NewSpanReader() (spanstore.Reader, error) {
	return esSpanstore.NewSpanReader(e.client, e.logger, e.maxLookback, e.metricsFactory), nil
}

func (e *esBuilder) NewDependencyReader() (dependencystore.Reader, error) {
	return esDependencyStore.NewDependencyStore(e.client, e.logger), nil
}
