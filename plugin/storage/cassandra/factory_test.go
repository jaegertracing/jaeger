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

package cassandra

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	cassCfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.Factory = new(Factory)
var _ storage.ArchiveFactory = new(Factory)

type mockSessionBuilder struct {
	session *mocks.Session
	err     error
}

func newMockSessionBuilder(session *mocks.Session, err error) *mockSessionBuilder {
	return &mockSessionBuilder{
		session: session,
		err:     err,
	}
}

func (m *mockSessionBuilder) NewSession(*zap.Logger) (cassandra.Session, error) {
	return m.session, m.err
}

func TestCassandraFactory(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--cassandra-archive.enabled=true", "--cassandra.enable-dependencies-v2=true"})
	f.InitFromViper(v)

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.primaryConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.primaryConfig = newMockSessionBuilder(session, nil)
	f.archiveConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.archiveConfig = nil
	assert.NoError(t, f.Initialize(metrics.NullFactory, logger))
	assert.Contains(t, logBuf.String(), "Cassandra archive storage configuration is empty, skipping")

	_, err := f.CreateSpanReader()
	assert.NoError(t, err)

	_, err = f.CreateSpanWriter()
	assert.NoError(t, err)

	_, err = f.CreateDependencyReader()
	assert.NoError(t, err)

	_, err = f.CreateArchiveSpanReader()
	assert.EqualError(t, err, "archive storage not configured")

	_, err = f.CreateArchiveSpanWriter()
	assert.EqualError(t, err, "archive storage not configured")

	f.archiveConfig = newMockSessionBuilder(session, nil)
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err = f.CreateArchiveSpanReader()
	assert.NoError(t, err)

	_, err = f.CreateArchiveSpanWriter()
	assert.NoError(t, err)

	assert.NoError(t, f.Close())
}

func TestExclusiveWhitelistBlacklist(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--cassandra-archive.enabled=true",
		"--cassandra.enable-dependencies-v2=true",
		"--cassandra.index.tag-whitelist=a,b,c",
		"--cassandra.index.tag-blacklist=a,b,c"})
	f.InitFromViper(v)

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.primaryConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.primaryConfig = newMockSessionBuilder(session, nil)
	f.archiveConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.archiveConfig = nil
	assert.NoError(t, f.Initialize(metrics.NullFactory, logger))
	assert.Contains(t, logBuf.String(), "Cassandra archive storage configuration is empty, skipping")

	_, err := f.CreateSpanWriter()
	assert.EqualError(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")

	f.archiveConfig = &mockSessionBuilder{}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err = f.CreateArchiveSpanWriter()
	assert.EqualError(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")
}

func TestWriterOptions(t *testing.T) {
	opts := NewOptions("cassandra")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tag-whitelist=a,b,c"})
	opts.InitFromViper(v)

	options, _ := writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tag-blacklist=a,b,c"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tags=false"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tags=false", "--cassandra.index.tag-blacklist=a,b,c"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{""})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 0)
}

func TestInitFromOptions(t *testing.T) {
	f := NewFactory()
	o := NewOptions("foo", archiveStorageConfig)
	o.others[archiveStorageConfig].Enabled = true
	f.InitFromOptions(o)
	assert.Equal(t, o, f.Options)
	assert.Equal(t, o.GetPrimary(), f.primaryConfig)
	assert.Equal(t, o.Get(archiveStorageConfig), f.archiveConfig)
}

func TestNewOptions(t *testing.T) {
	primaryCfg := cassCfg.Configuration{Keyspace: "primary"}
	archiveCfg := cassCfg.Configuration{Keyspace: "archive"}
	o := NewOptionsFromConfig(primaryCfg, archiveCfg)
	assert.Equal(t, primaryCfg, o.Primary.Configuration)
	assert.Equal(t, primaryStorageConfig, o.Primary.namespace)
	assert.Equal(t, 1, len(o.others))
	assert.Equal(t, archiveCfg, o.others[archiveStorageConfig].Configuration)
	assert.Equal(t, archiveStorageConfig, o.others[archiveStorageConfig].namespace)
}
