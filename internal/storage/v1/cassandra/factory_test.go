// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
)

func TestCreateSpanReaderError(t *testing.T) {
	f := NewFactory()
	_, err := f.CreateSpanReader()
	require.ErrorContains(t, err, "not implemented")
}

func TestConfigureFromOptions(t *testing.T) {
	f := NewFactory()
	o := NewOptions()
	f.ConfigureFromOptions(o)
	assert.Equal(t, o, f.Options)
	assert.Equal(t, o.GetConfig(), f.config)
}

func TestFactory_Purge(t *testing.T) {
	f := NewFactory()
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.session = session

	err := f.Purge(context.Background())
	require.NoError(t, err)

	session.AssertCalled(t, "Query", mock.AnythingOfType("string"), mock.Anything)
	query.AssertCalled(t, "Exec")
}

func TestNewSessionErrors(t *testing.T) {
	t.Run("NewCluster error", func(t *testing.T) {
		cfg := &config.Configuration{
			Connection: config.Connection{
				TLS: configtls.ClientConfig{
					Config: configtls.Config{
						CAFile: "foobar",
					},
				},
			},
		}
		_, err := NewSession(cfg)
		require.ErrorContains(t, err, "failed to load TLS config")
	})
	t.Run("CreateSession error", func(t *testing.T) {
		cfg := &config.Configuration{}
		_, err := NewSession(cfg)
		require.ErrorContains(t, err, "no hosts provided")
	})
	t.Run("CreateSession error with schema", func(t *testing.T) {
		cfg := &config.Configuration{
			Schema: config.Schema{
				CreateSchema: true,
			},
		}
		_, err := NewSession(cfg)
		require.ErrorContains(t, err, "no hosts provided")
	})
}

func TestIsArchiveCapable(t *testing.T) {
	tests := []struct {
		name           string
		archiveEnabled bool
		expected       bool
	}{
		{
			name:           "archive capable",
			archiveEnabled: true,
			expected:       true,
		},
		{
			name:           "not capable",
			archiveEnabled: false,
			expected:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := &Factory{
				Options: &Options{
					ArchiveEnabled: test.archiveEnabled,
				},
			}
			result := factory.IsArchiveCapable()
			require.Equal(t, test.expected, result)
		})
	}
}
