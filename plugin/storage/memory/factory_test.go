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

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/fork"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.Factory = new(Factory)

func TestMemoryStorageFactory(t *testing.T) {
	f := NewFactory()
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	assert.NotNil(t, f.store)
	reader, err := f.CreateSpanReader()
	assert.NoError(t, err)
	assert.Equal(t, f.store, reader)
	writer, err := f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store, writer)
	depReader, err := f.CreateDependencyReader()
	assert.NoError(t, err)
	assert.Equal(t, f.store, depReader)
}

func TestWithConfiguration(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--memory.max-traces=100"})
	f.InitFromViper(v)
	assert.Equal(t, f.options.Configuration.MaxTraces, 100)
}

func TestInitFromOptions(t *testing.T) {
	o := Options{}
	f := Factory{}
	f.InitFromOptions(o)
	assert.Equal(t, o, f.options)
}

func TestPublishOpts(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--memory.max-traces=100"})
	f.InitFromViper(v)

	baseMetrics := metricstest.NewFactory(time.Second)
	forkFactory := metricstest.NewFactory(time.Second)
	metricsFactory := fork.New("internal", forkFactory, baseMetrics)
	assert.NoError(t, f.Initialize(metricsFactory, zap.NewNop()))

	forkFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "internal." + limit,
		Value: f.options.Configuration.MaxTraces,
	})
}
