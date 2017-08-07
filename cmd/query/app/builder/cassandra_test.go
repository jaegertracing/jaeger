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

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/pkg/cassandra"
	"github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
)

type mockSessionBuilder struct {
}

func (*mockSessionBuilder) NewSession() (cassandra.Session, error) {
	return &mocks.Session{}, nil
}

func TestNewBuilderFailure(t *testing.T) {
	sFlags := &flags.SharedFlags{}
	sb := newStorageBuilder()
	err := sb.newCassandraBuilder(&config.Configuration{}, sFlags.DependencyStorage.DataFrequency)
	require.Error(t, err)
	assert.Nil(t, sb.SpanReader)
	assert.Nil(t, sb.DependencyReader)
}

func TestNewBuilderSuccess(t *testing.T) {
	sFlags := &flags.SharedFlags{}

	sb := newStorageBuilder()
	err := sb.newCassandraBuilder(&mockSessionBuilder{}, sFlags.DependencyStorage.DataFrequency)
	require.NoError(t, err)
	assert.NotNil(t, sb.SpanReader)
	assert.NotNil(t, sb.DependencyReader)
}
