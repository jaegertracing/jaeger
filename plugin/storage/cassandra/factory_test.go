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
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	mockcql "github.com/gocql/gocql"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	cassandraCfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

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
	command.ParseFlags([]string{"--cassandra-archive.enabled=true"})
	f.InitFromViper(v, zap.NewNop())

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.primaryConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.primaryConfig = newMockSessionBuilder(session, nil)
	f.archiveConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.archiveConfig = nil
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	assert.Contains(t, logBuf.String(), "Cassandra archive storage configuration is empty, skipping")

	_, err := f.CreateSpanReader()
	require.NoError(t, err)

	_, err = f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateArchiveSpanReader()
	require.EqualError(t, err, "archive storage not configured")

	_, err = f.CreateArchiveSpanWriter()
	require.EqualError(t, err, "archive storage not configured")

	f.archiveConfig = newMockSessionBuilder(session, nil)
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err = f.CreateArchiveSpanReader()
	require.NoError(t, err)

	_, err = f.CreateArchiveSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateLock()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(0)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestExclusiveWhitelistBlacklist(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--cassandra-archive.enabled=true",
		"--cassandra.index.tag-whitelist=a,b,c",
		"--cassandra.index.tag-blacklist=a,b,c",
	})
	f.InitFromViper(v, zap.NewNop())

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.primaryConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.primaryConfig = newMockSessionBuilder(session, nil)
	f.archiveConfig = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.archiveConfig = nil
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	assert.Contains(t, logBuf.String(), "Cassandra archive storage configuration is empty, skipping")

	_, err := f.CreateSpanWriter()
	require.EqualError(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")

	f.archiveConfig = &mockSessionBuilder{}
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err = f.CreateArchiveSpanWriter()
	require.EqualError(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")
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
	assert.Empty(t, options)
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

// func TestCassandraStorageFactoryWithConfig(t *testing.T) {
// 	mockServerResponse := []byte{}
// 	listener, err := net.Listen("tcp", "127.0.0.1:0") // Use port 0 to get an available port
// 	require.NoError(t, err)

// 	var wg sync.WaitGroup
// 	wg.Add(1)

// 	go func() {
// 		defer wg.Done()
// 		conn, err := listener.Accept()
// 		require.NoError(t, err)
// 		defer conn.Close()

// 		_, err = conn.Write(mockServerResponse)
// 		require.NoError(t, err)
// 	}()
// 	serverURL := listener.Addr().String()
// 	serverURL = serverURL[len("http://"):]
// 	link, portStr, err := net.SplitHostPort(serverURL)
// 	require.NoError(t, err)
// 	port, err := strconv.Atoi(portStr)
// 	require.NoError(t, err)
// 	cfg := cassandraCfg.Configuration{
// 		Servers:      []string{link},
// 		Keyspace:     "test",
// 		ProtoVersion: 3,
// 		Port:         port,
// 	}
// 	factory, err := NewFactoryWithConfig(cfg, metrics.NullFactory, zap.NewNop())
// 	require.NoError(t, err)
// 	defer factory.Close()
// }

func TestCassandraStorageFactoryWithConfig(t *testing.T) {
	cfg := cassandraCfg.Configuration{}
	_, err := NewFactoryWithConfig(cfg, metrics.NullFactory, zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "servers not found")
	cluster := mockcql.NewCluster("192.168.1.1")
	session, err := cluster.CreateSession()
	require.NoError(t, err)
	defer session.Close()

	lis, err := net.Listen("tcp", "192.168.1.1:0")
	require.NoError(t, err, "failed to listen")

	cfg = cassandraCfg.Configuration{
		Servers: []string{lis.Addr().String()},
	}
	f, err := NewFactoryWithConfig(cfg, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, f.Close())
	// var (
	// 	session = &mocks.Session{}
	// 	query   = &mocks.Query{}
	// )
	// session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	// query.On("Exec").Return(nil)
	// f.primaryConfig = newMockSessionBuilder(session, nil)

}

func TestConfigurationValidation(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     cassandraCfg.Configuration
		wantErr bool
	}{
		{
			name: "valid configuration",
			cfg: cassandraCfg.Configuration{
				Servers: []string{"http://localhost:9200"},
			},
			wantErr: false,
		},
		{
			name:    "missing servers",
			cfg:     cassandraCfg.Configuration{},
			wantErr: true,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			if test.wantErr {
				require.Error(t, err)
				_, err = NewFactoryWithConfig(test.cfg, metrics.NullFactory, zap.NewNop())
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// GoRoutine Leak in this test
func TestCassandraFactoryWithConfigError(t *testing.T) {
	cfg := cassandraCfg.Configuration{
		Servers: []string{"http://badurl"},
	}
	_, err := NewFactoryWithConfig(cfg, metrics.NullFactory, zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "gocql: unable to create session: strconv.Atoi: parsing \"//badurl\": invalid syntax")
	err = cfg.Close()
	require.NoError(t, err)
}
