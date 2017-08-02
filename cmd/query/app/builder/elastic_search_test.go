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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/es/config"
)

func withESBuilder(t *testing.T, f func(builder *esBuilder)) {
	cBuilder, err := newESBuilder(&mockElasticBuilder{}, zap.NewNop(), metrics.NullFactory, time.Hour)
	require.NoError(t, err)
	f(cBuilder)
}

func TestNewESBuilder(t *testing.T) {
	withESBuilder(t, func(esBuilder *esBuilder) {
		assert.NotNil(t, esBuilder)
		assert.NotNil(t, esBuilder.logger)
		assert.NotNil(t, esBuilder.client)
		assert.NotNil(t, esBuilder.client)
	})
}

func TestESBuilderFailure(t *testing.T) {
	cfg := &config.Configuration{
		Servers: []string{"127.0.0.1"},
	}
	cBuilder, error := newESBuilder(cfg, zap.NewNop(), metrics.NullFactory, time.Hour)
	require.Error(t, error)
	assert.Nil(t, cBuilder)
}

func TestESBuilderSuccesses(t *testing.T) {
	withESBuilder(t, func(esBuilder *esBuilder) {
		spanReader, err := esBuilder.NewSpanReader()
		assert.NoError(t, err)
		assert.NotNil(t, spanReader)
		depReader, err := esBuilder.NewDependencyReader()
		assert.NoError(t, err)
		assert.NotNil(t, depReader)
	})
}
