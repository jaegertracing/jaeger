// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
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

func TestNewCluster_PreservesDriverTimeoutDefaultsWhenUnset(t *testing.T) {
	cfg := Configuration{
		Connection: Connection{
			Servers: []string{"127.0.0.1:9042"},
			// Timeout left at zero as when connection.timeout is omitted from YAML.
		},
		Schema: Schema{
			CompactionWindow: time.Minute,
		},
		// Query.Timeout left at zero as when query.timeout is omitted from YAML.
	}
	require.NoError(t, cfg.Validate())
	cl, err := cfg.NewCluster()
	require.NoError(t, err)
	assert.Equal(t, 11*time.Second, cl.Timeout, "gocql NewCluster default query timeout")
	assert.Equal(t, 11*time.Second, cl.ConnectTimeout, "gocql NewCluster default connect timeout")
}

func TestNewCluster_AppliesTimeoutsWhenSet(t *testing.T) {
	cfg := Configuration{
		Connection: Connection{
			Servers: []string{"127.0.0.1:9042"},
			Timeout: 60 * time.Second,
		},
		Schema: Schema{
			CompactionWindow: time.Minute,
		},
		Query: Query{
			Timeout: 45 * time.Second,
		},
	}
	require.NoError(t, cfg.Validate())
	cl, err := cfg.NewCluster()
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, cl.Timeout)
	assert.Equal(t, 60*time.Second, cl.ConnectTimeout)
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
