// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptions(t *testing.T) {
	opts := NewOptions("foo")
	primary := opts.GetPrimary()
	assert.NotEmpty(t, primary.Schema.Keyspace)
	assert.NotEmpty(t, primary.Connection.Servers)
	assert.Equal(t, 2, primary.Connection.ConnectionsPerHost)

	aux := opts.Get("archive")
	assert.Nil(t, aux)

	assert.NotNil(t, opts.others["archive"])
	opts.others["archive"].Enabled = true
	aux = opts.Get("archive")
	require.NotNil(t, aux)
	assert.Equal(t, primary.Schema.Keyspace, aux.Schema.Keyspace)
	assert.Equal(t, primary.Connection.Servers, aux.Connection.Servers)
	assert.Equal(t, primary.Connection.ConnectionsPerHost, aux.Connection.ConnectionsPerHost)
	assert.Equal(t, primary.Connection.ReconnectInterval, aux.Connection.ReconnectInterval)
}

func TestOptionsWithFlags(t *testing.T) {
	opts := NewOptions("cas", "cas-aux")
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
		// enable aux with a couple overrides
		"--cas-aux.enabled=true",
		"--cas-aux.keyspace=jaeger-archive",
		"--cas-aux.servers=3.3.3.3, 4.4.4.4",
		"--cas-aux.username=username",
		"--cas-aux.password=password",
		"--cas-aux.basic.allowed-authenticators=org.apache.cassandra.auth.PasswordAuthenticator,com.ericsson.bss.cassandra.ecaudit.auth.AuditAuthenticator",
	})
	opts.InitFromViper(v)

	primary := opts.GetPrimary()
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

	aux := opts.Get("cas-aux")
	require.NotNil(t, aux)
	assert.Equal(t, "jaeger-archive", aux.Schema.Keyspace)
	assert.Equal(t, []string{"3.3.3.3", "4.4.4.4"}, aux.Connection.Servers)
	assert.Equal(t, []string{"org.apache.cassandra.auth.PasswordAuthenticator", "com.ericsson.bss.cassandra.ecaudit.auth.AuditAuthenticator"}, aux.Connection.Authenticator.Basic.AllowedAuthenticators)
	assert.Equal(t, 42, aux.Connection.ConnectionsPerHost)
	assert.Equal(t, 42, aux.Query.MaxRetryAttempts)
	assert.Equal(t, 42*time.Second, aux.Query.Timeout)
	assert.Equal(t, 42*time.Second, aux.Connection.ReconnectInterval)
	assert.Equal(t, 4242, aux.Connection.Port)
	assert.Equal(t, "", aux.Query.Consistency, "aux storage does not inherit consistency from primary")
	assert.Equal(t, 3, aux.Connection.ProtoVersion)
	assert.Equal(t, 42*time.Second, aux.Connection.SocketKeepAlive)
}

func TestDefaultTlsHostVerify(t *testing.T) {
	opts := NewOptions("cas")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--cas.tls.enabled=true",
	})
	opts.InitFromViper(v)

	primary := opts.GetPrimary()
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
