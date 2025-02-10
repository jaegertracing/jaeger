// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptions(t *testing.T) {
	opts := NewOptions("foo")
	primary := opts.GetConfig()
	assert.NotEmpty(t, primary.Schema.Keyspace)
	assert.NotEmpty(t, primary.Connection.Servers)
	assert.Equal(t, 2, primary.Connection.ConnectionsPerHost)
}

func TestOptionsWithFlags(t *testing.T) {
	opts := NewOptions("cas")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--cas.keyspace=jaeger",
		"--cas.local-dc=mojave",
		"--cas.servers=1.1.1.1, 2.2.2.2",
		"--cas.connections-per-host=42",
		"--cas.reconnect-interval=42s",
		"--cas.max-retry-attempts=42",
		"--cas.timeout=42s",
		"--cas.port=4242",
		"--cas.consistency=ONE",
		"--cas.proto-version=3",
		"--cas.socket-keep-alive=42s",
		"--cas.index.tag-blacklist=blerg, blarg,blorg ",
		"--cas.index.tag-whitelist=flerg, flarg,florg ",
		"--cas.index.tags=true",
		"--cas.index.process-tags=false",
		"--cas.basic.allowed-authenticators=org.apache.cassandra.auth.PasswordAuthenticator,com.datastax.bdp.cassandra.auth.DseAuthenticator",
		"--cas.username=username",
		"--cas.password=password",
	})
	opts.InitFromViper(v)

	primary := opts.GetConfig()
	assert.Equal(t, "jaeger", primary.Schema.Keyspace)
	assert.Equal(t, "mojave", primary.Connection.LocalDC)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Connection.Servers)
	assert.Equal(t, []string{"org.apache.cassandra.auth.PasswordAuthenticator", "com.datastax.bdp.cassandra.auth.DseAuthenticator"}, primary.Connection.Authenticator.Basic.AllowedAuthenticators)
	assert.Equal(t, "ONE", primary.Query.Consistency)
	assert.Equal(t, []string{"blerg", "blarg", "blorg"}, opts.TagIndexBlacklist())
	assert.Equal(t, []string{"flerg", "flarg", "florg"}, opts.TagIndexWhitelist())
	assert.True(t, opts.Index.Tags)
	assert.False(t, opts.Index.ProcessTags)
	assert.True(t, opts.Index.Logs)
}

func TestDefaultTlsHostVerify(t *testing.T) {
	opts := NewOptions("cas")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--cas.tls.enabled=true",
	})
	opts.InitFromViper(v)

	primary := opts.GetConfig()
	assert.False(t, primary.Connection.TLS.InsecureSkipVerify)
}

func TestEmptyBlackWhiteLists(t *testing.T) {
	opts := NewOptions("cas")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Empty(t, opts.TagIndexBlacklist())
	assert.Empty(t, opts.TagIndexWhitelist())
}
