// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ReturnsErrorWhenInvalid(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Configuration
	}{
		{
			name: "missing required fields",
			cfg:  &Configuration{},
		},
		{
			name: "require fields in invalid format",
			cfg: &Configuration{
				Connection: Connection{
					Servers: []string{"not a url"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			require.Error(t, err)
		})
	}
}

func TestValidate_DoesNotReturnErrorWhenRequiredFieldsSet(t *testing.T) {
	cfg := Configuration{
		Connection: Connection{
			Servers: []string{"localhost:9200"},
		},
		Schema: Schema{
			CompactionWindow: time.Minute,
		},
	}

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestNewClusterWithDefaults(t *testing.T) {
	cfg := DefaultConfiguration()
	cl, err := cfg.NewCluster()
	require.NoError(t, err)
	assert.NotEmpty(t, cl.Keyspace)
}

func TestNewClusterWithOverrides(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.Query.Consistency = "LOCAL_QUORUM"
	cfg.Connection.LocalDC = "local_dc"
	cfg.Connection.Authenticator.Basic.Username = "username"
	cfg.Connection.Authenticator.Basic.Password = "password"
	cfg.Connection.TLS.Insecure = false
	cfg.Connection.DisableAutoDiscovery = true
	cl, err := cfg.NewCluster()
	require.NoError(t, err)
	assert.NotEmpty(t, cl.Keyspace)
	assert.Equal(t, gocql.LocalQuorum, cl.Consistency)
	assert.NotNil(t, cl.PoolConfig.HostSelectionPolicy, "local_dc")
	require.IsType(t, gocql.PasswordAuthenticator{}, cl.Authenticator)
	auth := cl.Authenticator.(gocql.PasswordAuthenticator)
	assert.Equal(t, "username", auth.Username)
	assert.Equal(t, "password", auth.Password)
	assert.NotNil(t, cl.SslOpts)
	assert.True(t, cl.DisableInitialHostLookup)
}

func TestApplyDefaults(t *testing.T) {
	cfg1 := DefaultConfiguration()
	cfg2 := Configuration{}
	cfg2.ApplyDefaults(&cfg1)
	assert.Equal(t, cfg2.Schema, cfg1.Schema)
	assert.Equal(t, cfg2.Query, cfg1.Query)
	assert.NotEqual(t, cfg2.Connection.Servers, cfg1.Connection.Servers, "servers not copied")
	cfg1.Connection.Servers = nil
	assert.Equal(t, cfg2.Connection, cfg1.Connection)
}

func TestToString(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.Schema.Keyspace = "test"
	s := cfg.String()
	assert.Contains(t, s, "Keyspace:test")
}

func TestConfigSchemaValidation(t *testing.T) {
	cfg := DefaultConfiguration()
	err := cfg.Validate()
	require.NoError(t, err)

	cfg.Schema.TraceTTL = time.Millisecond
	err = cfg.Validate()
	require.Error(t, err)

	cfg.Schema.TraceTTL = time.Second
	cfg.Schema.CompactionWindow = time.Minute - 1
	err = cfg.Validate()
	require.Error(t, err)

	cfg.Schema.CompactionWindow = time.Minute
	cfg.Schema.DependenciesTTL = time.Second - 1
	err = cfg.Validate()
	require.Error(t, err)
}
