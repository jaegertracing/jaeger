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
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/pkg/es/config"
	"github.com/uber/jaeger/pkg/es/mocks"
)

type mockEsBuilder struct {
	config.Configuration
}

func (mck *mockEsBuilder) NewClient() (es.Client, error) {
	return &mocks.Client{}, nil
}

func TestNewESBuilderSuccess(t *testing.T) {
	cBuilder, err := newESBuilder(&mockEsBuilder{}, zap.NewNop(), metrics.NullFactory)
	require.NoError(t, err)
	assert.NotNil(t, cBuilder)
	reader, err := cBuilder.NewSpanReader()
	require.NoError(t, err)
	assert.NotNil(t, reader)
	depenReader, err := cBuilder.NewDependencyReader()
	require.NoError(t, err)
	assert.NotNil(t, depenReader)
}

func TestNewESBuilderFailure(t *testing.T) {
	cBuilder, err := newESBuilder(&config.Configuration{}, zap.NewNop(), metrics.NullFactory)
	require.Error(t, err)
	assert.Nil(t, cBuilder)
}
