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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptions(t *testing.T) {
	opts := NewOptions("foo")
	primary := opts.GetPrimary()
	assert.NotEmpty(t, primary.Keyspace)
	assert.NotEmpty(t, primary.Servers)
	assert.Equal(t, 2, primary.ConnectionsPerHost)

	aux := opts.Get("archive")
	assert.Nil(t, aux)

	assert.NotNil(t, opts.others["archive"])
	opts.others["archive"].Enabled = true
	aux = opts.Get("archive")
	require.NotNil(t, aux)
	assert.Equal(t, primary.Keyspace, aux.Keyspace)
	assert.Equal(t, primary.Servers, aux.Servers)
	assert.Equal(t, primary.ConnectionsPerHost, aux.ConnectionsPerHost)
	assert.Equal(t, primary.ReconnectInterval, aux.ReconnectInterval)
}

func TestOptionsWithFlags(t *testing.T) {
	opts := NewOptions("cas", "cas-aux")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--cas.keyspace=jaeger",
		"--cas.local-dc=mojave",
		"--cas.servers=1.1.1.1,2.2.2.2",
		"--cas.connections-per-host=42",
		"--cas.reconnect-interval=42s",
		"--cas.max-retry-attempts=42",
		"--cas.timeout=42s",
		"--cas.port=4242",
		"--cas.consistency=ONE",
		"--cas.proto-version=3",
		"--cas.socket-keep-alive=42s",
		// enable aux with a couple overrides
		"--cas-aux.enabled=true",
		"--cas-aux.keyspace=jaeger-archive",
		"--cas-aux.servers=3.3.3.3,4.4.4.4",
	})
	opts.InitFromViper(v)

	primary := opts.GetPrimary()
	assert.Equal(t, "jaeger", primary.Keyspace)
	assert.Equal(t, "mojave", primary.LocalDC)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	assert.Equal(t, "ONE", primary.Consistency)

	aux := opts.Get("cas-aux")
	require.NotNil(t, aux)
	assert.Equal(t, "jaeger-archive", aux.Keyspace)
	assert.Equal(t, []string{"3.3.3.3", "4.4.4.4"}, aux.Servers)
	assert.Equal(t, 42, aux.ConnectionsPerHost)
	assert.Equal(t, 42, aux.MaxRetryAttempts)
	assert.Equal(t, 42*time.Second, aux.Timeout)
	assert.Equal(t, 42*time.Second, aux.ReconnectInterval)
	assert.Equal(t, 4242, aux.Port)
	assert.Equal(t, "", aux.Consistency, "aux storage does not inherit consistency from primary")
	assert.Equal(t, 3, aux.ProtoVersion)
	assert.Equal(t, 42*time.Second, aux.SocketKeepAlive)
}
