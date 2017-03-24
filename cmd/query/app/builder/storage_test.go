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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger-lib/metrics"
	basicB "github.com/uber/jaeger/cmd/builder"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

func TestNewCassandraSuccess(t *testing.T) {
	sBuilder, err := NewStorageBuilder(
		basicB.Options.LoggerOption(zap.New(zap.NullEncoder())),
		basicB.Options.MetricsFactoryOption(metrics.NullFactory),
		basicB.Options.CassandraOption(&cascfg.Configuration{
			Servers: []string{"127.0.0.1"},
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewCassandraFailure(t *testing.T) {
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"test", "--span-storage.type=sneh"}
	sBuilder, err := NewStorageBuilder()
	assert.EqualError(t, err, "Storage Type is not supported")
	assert.Nil(t, sBuilder)

	os.Args = []string{"test", "--span-storage.type=cassandra"}
	sBuilder, err = NewStorageBuilder()
	assert.EqualError(t, err, "Cassandra not configured")
	assert.Nil(t, sBuilder)
}

func TestNewMemorySuccess(t *testing.T) {
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"test", "--span-storage.type=memory"}
	sBuilder, err := NewStorageBuilder(basicB.Options.MemoryStoreOption(memory.NewStore()))
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewMemoryFailure(t *testing.T) {
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"test", "--span-storage.type=memory"}
	sBuilder, err := NewStorageBuilder()
	assert.Error(t, err)
	assert.Nil(t, sBuilder)
}
