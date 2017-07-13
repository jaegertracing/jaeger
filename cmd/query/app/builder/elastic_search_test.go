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

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/es/config"
	"github.com/uber/jaeger/pkg/es/mocks"
)

func withESBuilder(f func(builder *esBuilder)) {
	cfg := &config.Configuration{
		Servers: []string{"127.0.0.1"},
	}
	cBuilder := newESBuilder(cfg, zap.NewNop())
	f(cBuilder)
}

func TestNewESBuilder(t *testing.T) {
	withESBuilder(func(esBuilder *esBuilder) {
		assert.NotNil(t, esBuilder)
		assert.NotNil(t, esBuilder.logger)
		assert.NotNil(t, esBuilder.configuration)
		assert.Nil(t, esBuilder.client)
	})
}

func TestESBuilderFailure(t *testing.T) {
	withESBuilder(func(esBuilder *esBuilder) {
		esBuilder.configuration.Servers = []string{}
		spanReader, err := esBuilder.NewSpanReader()
		assert.Error(t, err)
		assert.Nil(t, spanReader)
		dependencyReader, err := esBuilder.NewDependencyReader()
		assert.Error(t, err)
		assert.Nil(t, dependencyReader)
	})
}

func TestESBuilderSuccesses(t *testing.T) {
	withESBuilder(func(esBuilder *esBuilder) {
		mockClient := mocks.Client{}
		esBuilder.client = &mockClient
		spanReader, err := esBuilder.NewSpanReader()
		assert.NoError(t, err)
		assert.NotNil(t, spanReader)
		depReader, err := esBuilder.NewDependencyReader()
		assert.NoError(t, err)
		assert.NotNil(t, depReader)
	})
}
