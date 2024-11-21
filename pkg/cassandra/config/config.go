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
	Query      Query      `mapstructure:"query"`
}

type Connection struct {
	// Servers contains a list of hosts that are used to connect to the cluster.
	Servers []string `mapstructure:"servers" valid:"required,url"`
	// LocalDC contains the name of the local Data Center (DC) for DC-aware host selection
	LocalDC string `mapstructure:"local_dc"`
	// The port used when dialing to a cluster.
	Port int `mapstructure:"port"`
	// DisableAutoDiscovery, if set to true, will disable the cluster's auto-discovery features.
	DisableAutoDiscovery bool `mapstructure:"disable_auto_discovery"`
	// ConnectionsPerHost contains the maximum number of open connections for each host on the cluster.
	ConnectionsPerHost int `mapstructure:"connections_per_host"`
	// ReconnectInterval contains the regular interval after which the driver tries to connect to
	// nodes that are down.
	ReconnectInterval time.Duration `mapstructure:"reconnect_interval"`
	// SocketKeepAlive contains the keep alive period for the default dialer to the cluster.
	SocketKeepAlive time.Duration `mapstructure:"socket_keep_alive"`
	// TLS contains the TLS configuration for the connection to the cluster.
	TLS configtls.ClientConfig `mapstructure:"tls"`
	// Timeout contains the maximum time spent to connect to a cluster.
	Timeout time.Duration `mapstructure:"timeout"`
	// Authenticator contains the details of the authentication mechanism that is used for
	// connecting to a cluster.
	Authenticator Authenticator `mapstructure:"auth"`
	// ProtoVersion contains the version of the native protocol to use when connecting to a cluster.
	ProtoVersion int `mapstructure:"proto_version"`
}

type Schema struct {
	// Keyspace contains the namespace where Jaeger data will be stored.
	Keyspace string `mapstructure:"keyspace"`
	// DisableCompression, if set to true, disables the use of the default Snappy Compression
	// while connecting to the Cassandra Cluster. This is useful for connecting to clusters, like Azure Cosmos DB,
	// that do not support SnappyCompression.
	DisableCompression bool `mapstructure:"disable_compression"`
	// CreateSchema tells if the schema ahould be created during session initialization based on the configs provided
	CreateSchema bool `mapstructure:"create" valid:"optional"`
	// Datacenter is the name for network topology
	Datacenter string `mapstructure:"datacenter" valid:"optional"`
	// TraceTTL is Time To Live (TTL) for the trace data. Should at least be 1 second
	TraceTTL time.Duration `mapstructure:"trace_ttl" valid:"optional,cassandraTTLValidation"`
	// DependenciesTTL is Time To Live (TTL) for dependencies data. Should at least be 1 second
	DependenciesTTL time.Duration `mapstructure:"dependencies_ttl" valid:"optional,cassandraTTLValidation"`
	// Replication factor for the db
	ReplicationFactor int `mapstructure:"replication_factor" valid:"optional"`
	// CasVersion is version of cassandra used
	CasVersion int `mapstructure:"cas_version" valid:"optional"`
	// CompactionWindow of format tells the compaction window of the db. Should atleast be 1 minute
	CompactionWindow time.Duration `mapstructure:"compaction_window" valid:"optional,isDurationGreaterThanOrEqualMinute"`
}

type Query struct {
	// Timeout contains the maximum time spent executing a query.
	Timeout time.Duration `mapstructure:"timeout"`
	// MaxRetryAttempts indicates the maximum number of times a query will be retried for execution.
	MaxRetryAttempts int `mapstructure:"max_retry_attempts"`
	// Consistency specifies the consistency level which needs to be satisified before responding
	// to a query.
	Consistency string `mapstructure:"consistency"`
}

// Authenticator holds the authentication properties needed to connect to a Cassandra cluster.
type Authenticator struct {
	Basic BasicAuthenticator `mapstructure:"basic"`
	// TODO: add more auth types
}

// BasicAuthenticator holds the username and password for a password authenticator for a Cassandra cluster.
type BasicAuthenticator struct {
	Username              string   `mapstructure:"username"`
	Password              string   `mapstructure:"password" json:"-"`
	AllowedAuthenticators []string `mapstructure:"allowed_authenticators"`
}

func DefaultConfiguration() Configuration {
	return Configuration{
		Schema: Schema{
			CreateSchema:      false,
			Keyspace:          "jaeger_v1_dc1",
			Datacenter:        "test",
			TraceTTL:          2 * 24 * time.Hour,
			DependenciesTTL:   2 * 24 * time.Hour,
			ReplicationFactor: 1,
			CasVersion:        4,
			CompactionWindow:  time.Minute,
		},
		Connection: Connection{
			Servers:            []string{"127.0.0.1"},
			Port:               9042,
			ProtoVersion:       4,
			ConnectionsPerHost: 2,
			ReconnectInterval:  60 * time.Second,
		},
		Query: Query{
			MaxRetryAttempts: 3,
		},
	}
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
	if c.Schema.Keyspace == "" {
		c.Schema.Keyspace = source.Schema.Keyspace
	}

	if c.Schema.Datacenter == "" {
		c.Schema.Datacenter = source.Schema.Datacenter
	}

	if c.Schema.TraceTTL == 0 {
		c.Schema.TraceTTL = source.Schema.TraceTTL
	}

	if c.Schema.DependenciesTTL == 0 {
		c.Schema.DependenciesTTL = source.Schema.DependenciesTTL
	}

	if c.Schema.ReplicationFactor == 0 {
		c.Schema.ReplicationFactor = source.Schema.ReplicationFactor
	}

	if c.Schema.CasVersion == 0 {
		c.Schema.CasVersion = source.Schema.CasVersion
	}

	if c.Schema.CompactionWindow == 0 {
		c.Schema.CompactionWindow = source.Schema.CompactionWindow
	}

	if c.Connection.ConnectionsPerHost == 0 {
		c.Connection.ConnectionsPerHost = source.Connection.ConnectionsPerHost
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
	if c.Query.MaxRetryAttempts == 0 {
		c.Query.MaxRetryAttempts = source.Query.MaxRetryAttempts
	}
	if c.Query.Timeout == 0 {
		c.Query.Timeout = source.Query.Timeout
	}
}

// SessionBuilder creates new cassandra.Session
type SessionBuilder interface {
	NewSession() (cassandra.Session, error)
}

func (c *Configuration) newSessionPrerequisites() error {
	if !c.Schema.CreateSchema {
		return nil
	}
	cluster, err := c.NewCluster()

	cluster.Keyspace = ""

	session, err := cluster.CreateSession()
	if err != nil {
		return err
	}

	wSession := gocqlw.WrapCQLSession(session)
	defer wSession.Close()

	return generateSchemaIfNotPresent(wSession, &c.Schema)
}

// NewSession creates a new Cassandra session
func (c *Configuration) NewSession() (cassandra.Session, error) {
	if err := c.newSessionPrerequisites(); err != nil {
		return nil, err
	}

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
	cluster.ConnectTimeout = c.Connection.Timeout
	cluster.ReconnectInterval = c.Connection.ReconnectInterval
	cluster.SocketKeepalive = c.Connection.SocketKeepAlive
	cluster.Timeout = c.Query.Timeout
	if c.Connection.ProtoVersion > 0 {
		cluster.ProtoVersion = c.Connection.ProtoVersion
	}
	if c.Query.MaxRetryAttempts > 1 {
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: c.Query.MaxRetryAttempts - 1}
	}
	if c.Connection.Port != 0 {
		cluster.Port = c.Connection.Port
	}

	if !c.Schema.DisableCompression {
		cluster.Compressor = gocql.SnappyCompressor{}
	}

	if c.Query.Consistency == "" {
		cluster.Consistency = gocql.LocalOne
	} else {
		cluster.Consistency = gocql.ParseConsistency(c.Query.Consistency)
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
	if !c.Connection.TLS.Insecure {
		tlsCfg, err := c.Connection.TLS.LoadTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}
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
	govalidator.CustomTypeTagMap.Set("cassandraTTLValidation", func(i interface{}, _ interface{}) bool {
		duration, ok := i.(time.Duration)
		if !ok {
			return false
		}
		// Either set to no-expiry using 0 or if set should be greater than or equal to second
		return duration == 0 || duration >= time.Second
	})

	govalidator.CustomTypeTagMap.Set("isDurationGreaterThanOrEqualMinute", func(i interface{}, _ interface{}) bool {
		duration, ok := i.(time.Duration)
		if !ok {
			return false
		}
		return duration >= time.Minute
	})

	_, err := govalidator.ValidateStruct(c)
	return err
}
