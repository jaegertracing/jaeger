// Copyright (c) 2019 The Jaeger Authors.
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

package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage-plugin.binary=noop-grpc-plugin",
		"--grpc-storage-plugin.configuration-file=config.json",
		"--grpc-storage-plugin.log-level=debug",
	})
	assert.NoError(t, err)
	opts.InitFromViper(v)

	assert.Equal(t, opts.Configuration.PluginBinary, "noop-grpc-plugin")
	assert.Equal(t, opts.Configuration.PluginConfigurationFile, "config.json")
	assert.Equal(t, opts.Configuration.PluginLogLevel, "debug")
}

func TestRemoteOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=true",
		"--grpc-storage.connection-timeout=60s",
	})
	assert.NoError(t, err)
	opts.InitFromViper(v)

	assert.Equal(t, opts.Configuration.PluginBinary, "")
	assert.Equal(t, opts.Configuration.RemoteServerAddr, "localhost:2001")
	assert.Equal(t, opts.Configuration.RemoteTLS.Enabled, true)
	assert.Equal(t, opts.Configuration.RemoteConnectTimeout, 60*time.Second)
}

func TestRemoteOptionsNoTLSWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=false",
		"--grpc-storage.connection-timeout=60s",
	})
	assert.NoError(t, err)
	opts.InitFromViper(v)

	assert.Equal(t, opts.Configuration.PluginBinary, "")
	assert.Equal(t, opts.Configuration.RemoteServerAddr, "localhost:2001")
	assert.Equal(t, opts.Configuration.RemoteTLS.Enabled, false)
	assert.Equal(t, opts.Configuration.RemoteConnectTimeout, 60*time.Second)
}

func TestFailedTLSFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.tls.enabled=false",
		"--grpc-storage.tls.cert=blah", // invalid unless tls.enabled=true
	})
	require.NoError(t, err)
	err = opts.InitFromViper(v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse gRPC storage TLS options")
}
