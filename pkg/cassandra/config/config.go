// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gocql/gocql"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	gocqlw "github.com/jaegertracing/jaeger/pkg/cassandra/gocql"
)

// Configuration describes the configuration properties needed to connect to a Cassandra cluster.
type Configuration struct {
	Schema     Schema     `mapstructure:"schema"`
	Connection Connection `mapstructure:"connection"`
}

type Connection struct {
	Servers              []string               `mapstructure:"servers" valid:"required,url" `
	LocalDC              string                 `mapstructure:"local_dc"`
	Port                 int                    `mapstructure:"port"`
	DisableAutoDiscovery bool                   `mapstructure:"disable_auto_discovery"`
	ConnectionsPerHost   int                    `mapstructure:"connections_per_host"`
	ReconnectInterval    time.Duration          `mapstructure:"reconnect_interval"`
	SocketKeepAlive      time.Duration          `mapstructure:"socket_keep_alive"`
	MaxRetryAttempts     int                    `mapstructure:"max_retry_attempts"`
	TLS                  configtls.ClientConfig `mapstructure:"tls"`
	QueryTimeout         time.Duration          `mapstructure:"query_timeout"`
	ConnectTimeout       time.Duration          `mapstructure:"connection_timeout"`
	Authenticator        Authenticator          `mapstructure:"auth"`
	ProtoVersion         int                    `mapstructure:"proto_version"`
	Consistency          string                 `mapstructure:"consistency"`
}

type Schema struct {
	Keyspace           string `mapstructure:"keyspace"`
	DisableCompression bool   `mapstructure:"disable_compression"`
}

// Authenticator holds the authentication properties needed to connect to a Cassandra cluster
type Authenticator struct {
	Basic BasicAuthenticator `mapstructure:"basic"`
	// TODO: add more auth types
}

// BasicAuthenticator holds the username and password for a password authenticator for a Cassandra cluster
type BasicAuthenticator struct {
	Username              string   `yaml:"username" mapstructure:"username"`
	Password              string   `yaml:"password" mapstructure:"password" json:"-"`
	AllowedAuthenticators []string `yaml:"allowed_authenticators" mapstructure:"allowed_authenticators"`
}

func DefaultConfiguration() Configuration {
	return Configuration{
		Connection: Connection{
			Servers:            []string{"127.0.0.1"},
			Port:               9042,
			MaxRetryAttempts:   3,
			ProtoVersion:       4,
			ConnectionsPerHost: 2,
			ReconnectInterval:  60 * time.Second,
		},
		Schema: Schema{
			Keyspace: "jaeger_v1_test",
		},
	}
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
	if c.Connection.ConnectionsPerHost == 0 {
		c.Connection.ConnectionsPerHost = source.Connection.ConnectionsPerHost
	}
	if c.Connection.MaxRetryAttempts == 0 {
		c.Connection.MaxRetryAttempts = source.Connection.MaxRetryAttempts
	}
	if c.Connection.QueryTimeout == 0 {
		c.Connection.QueryTimeout = source.Connection.QueryTimeout
	}
	if c.Connection.ReconnectInterval == 0 {
		c.Connection.ReconnectInterval = source.Connection.ReconnectInterval
	}
	if c.Connection.Port == 0 {
		c.Connection.Port = source.Connection.Port
	}
	if c.Connection.ProtoVersion == 0 {
		c.Connection.ProtoVersion = source.Connection.ProtoVersion
	}
	if c.Connection.SocketKeepAlive == 0 {
		c.Connection.SocketKeepAlive = source.Connection.SocketKeepAlive
	}

	if c.Schema.Keyspace == "" {
		c.Schema.Keyspace = source.Schema.Keyspace
	}
}

// SessionBuilder creates new cassandra.Session
type SessionBuilder interface {
	NewSession() (cassandra.Session, error)
}

// NewSession creates a new Cassandra session
func (c *Configuration) NewSession() (cassandra.Session, error) {
	cluster, err := c.NewCluster()
	if err != nil {
		return nil, err
	}
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}
	return gocqlw.WrapCQLSession(session), nil
}

// NewCluster creates a new gocql cluster from the configuration
func (c *Configuration) NewCluster() (*gocql.ClusterConfig, error) {
	cluster := gocql.NewCluster(c.Connection.Servers...)
	cluster.Keyspace = c.Schema.Keyspace
	cluster.NumConns = c.Connection.ConnectionsPerHost
	cluster.Timeout = c.Connection.QueryTimeout
	cluster.ConnectTimeout = c.Connection.ConnectTimeout
	cluster.ReconnectInterval = c.Connection.ReconnectInterval
	cluster.SocketKeepalive = c.Connection.SocketKeepAlive
	if c.Connection.ProtoVersion > 0 {
		cluster.ProtoVersion = c.Connection.ProtoVersion
	}
	if c.Connection.MaxRetryAttempts > 1 {
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: c.Connection.MaxRetryAttempts - 1}
	}
	if c.Connection.Port != 0 {
		cluster.Port = c.Connection.Port
	}

	if !c.Schema.DisableCompression {
		cluster.Compressor = gocql.SnappyCompressor{}
	}

	if c.Connection.Consistency == "" {
		cluster.Consistency = gocql.LocalOne
	} else {
		cluster.Consistency = gocql.ParseConsistency(c.Connection.Consistency)
	}

	fallbackHostSelectionPolicy := gocql.RoundRobinHostPolicy()
	if c.Connection.LocalDC != "" {
		fallbackHostSelectionPolicy = gocql.DCAwareRoundRobinPolicy(c.Connection.LocalDC)
	}
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(fallbackHostSelectionPolicy, gocql.ShuffleReplicas())

	if c.Connection.Authenticator.Basic.Username != "" && c.Connection.Authenticator.Basic.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username:              c.Connection.Authenticator.Basic.Username,
			Password:              c.Connection.Authenticator.Basic.Password,
			AllowedAuthenticators: c.Connection.Authenticator.Basic.AllowedAuthenticators,
		}
	}
	tlsCfg, err := c.Connection.TLS.LoadTLSConfig(context.Background())
	if err != nil {
		return nil, err
	}
	if !c.Connection.TLS.Insecure {
		cluster.SslOpts = &gocql.SslOptions{
			Config: tlsCfg,
		}
	}
	// If tunneling connection to C*, disable cluster autodiscovery features.
	if c.Connection.DisableAutoDiscovery {
		cluster.DisableInitialHostLookup = true
		cluster.IgnorePeerAddr = true
	}
	return cluster, nil
}

func (c *Configuration) String() string {
	return fmt.Sprintf("%+v", *c)
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}
