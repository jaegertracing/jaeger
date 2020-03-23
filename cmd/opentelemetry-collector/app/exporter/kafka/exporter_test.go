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

package kafka

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestNew(t *testing.T) {
	m := &mockProducerBuilder{}
	c := &Config{}
	exporter, err := create(m, c)
	require.Nil(t, err)
	assert.NotNil(t, exporter)
	m = &mockProducerBuilder{err: errors.New("failed to create")}
	exporter, err = create(m, c)
	assert.Error(t, err, "failed to create")
	assert.Nil(t, exporter)
}

type mockProducerBuilder struct {
	err error
}

func (m mockProducerBuilder) CreateSpanWriter() (spanstore.Writer, error) {
	return nil, m.err
}
func (mockProducerBuilder) CreateSpanReader() (spanstore.Reader, error) {
	return nil, nil
}
func (mockProducerBuilder) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, nil
}
func (mockProducerBuilder) Initialize(metrics.Factory, *zap.Logger) error {
	return nil
}
