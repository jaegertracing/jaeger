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

package config

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"github.com/uber/jaeger/pkg/cassandra"
	gocqlw "github.com/uber/jaeger/pkg/cassandra/gocql"
)

// Configuration describes the configuration properties needed to connect to a Cassandra cluster
type Configuration struct {
	Servers            []string      `validate:"nonzero"`
	Keyspace           string        `validate:"nonzero"`
	ConnectionsPerHost int           `validate:"min=1" yaml:"connections_per_host"`
	Timeout            time.Duration `validate:"min=500"`
	SocketKeepAlive    time.Duration `validate:"min=0" yaml:"socket_keep_alive"`
	MaxRetryAttempts   int           `validate:"min=0" yaml:"max_retry_attempt"`
	ProtoVersion       int           `yaml:"proto_version"`
	Consistency        string        `yaml:"consistency"`
	Port               int           `yaml:"port"`
}

// NewSession creates a new Cassandra session
func (c *Configuration) NewSession() (cassandra.Session, error) {
	cluster := c.NewCluster()
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}
	return gocqlw.WrapCQLSession(session), nil
}

// NewCluster creates a new gocql cluster from the configuration
func (c *Configuration) NewCluster() *gocql.ClusterConfig {
	cluster := gocql.NewCluster(c.Servers...)
	cluster.Keyspace = c.Keyspace
	cluster.NumConns = c.ConnectionsPerHost
	cluster.Timeout = c.Timeout
	cluster.SocketKeepalive = c.SocketKeepAlive
	if c.ProtoVersion > 0 {
		cluster.ProtoVersion = c.ProtoVersion
	}
	if c.MaxRetryAttempts > 1 {
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: c.MaxRetryAttempts - 1}
	}
	if c.Port != 0 {
		cluster.Port = c.Port
	}
	cluster.Compressor = gocql.SnappyCompressor{}
	if c.Consistency == "" {
		cluster.Consistency = gocql.LocalOne
	} else {
		cluster.Consistency = gocql.ParseConsistency(c.Consistency)
	}
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy())
	return cluster
}

func (c *Configuration) String() string {
	return fmt.Sprintf("%+v", *c)
}
