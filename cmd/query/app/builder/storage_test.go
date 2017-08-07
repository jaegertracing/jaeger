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

	basicB "github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/pkg/config"
	escfg "github.com/uber/jaeger/pkg/es/config"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

func TestNewCassandraSuccess(t *testing.T) {
	v, _ := config.Viperize(flags.AddFlags)
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(
		sFlags.SpanStorage.Type,
		sFlags.DependencyStorage.DataFrequency,
		basicB.Options.LoggerOption(zap.NewNop()),
		basicB.Options.MetricsFactoryOption(metrics.NullFactory),
		basicB.Options.CassandraSessionOption(&mockSessionBuilder{}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewCassandraFailure(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=sneh"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.EqualError(t, err, "Storage Type is not supported")
	assert.Nil(t, sBuilder)

	command.ParseFlags([]string{"test", "--span-storage.type=cassandra"})
	sFlags.InitFromViper(v)
	sBuilder, err = NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.EqualError(t, err, "Cassandra not configured")
	assert.Nil(t, sBuilder)
}

func TestNewMemorySuccess(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=memory"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency, basicB.Options.MemoryStoreOption(memory.NewStore()))
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewMemoryFailure(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=memory"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.Error(t, err)
	assert.Nil(t, sBuilder)
}

func TestNewElasticSuccess(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(
		sFlags.SpanStorage.Type,
		sFlags.DependencyStorage.DataFrequency,
		basicB.Options.LoggerOption(zap.NewNop()),
		basicB.Options.ElasticSearchOption(&escfg.Configuration{
			Servers: []string{"127.0.0.1"},
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewElasticFailure(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.EqualError(t, err, "ElasticSearch not configured")
	assert.Nil(t, sBuilder)
}
