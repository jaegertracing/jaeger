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
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

func TestOptionsWithFlags(t *testing.T) {
	v, command := config.Viperize(v1AddFlags, tenancy.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=foo:12345",
		"--multi-tenancy.header=x-scope-orgid",
	})
	require.NoError(t, err)
	var cfg Configuration
	require.NoError(t, v1InitFromViper(&cfg, v))

	assert.Equal(t, "foo:12345", cfg.RemoteServerAddr)
	assert.False(t, cfg.TenancyOpts.Enabled)
	assert.Equal(t, "x-scope-orgid", cfg.TenancyOpts.Header)
}

func TestRemoteOptionsWithFlags(t *testing.T) {
	v, command := config.Viperize(v1AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=true",
		"--grpc-storage.connection-timeout=60s",
	})
	require.NoError(t, err)
	var cfg Configuration
	require.NoError(t, v1InitFromViper(&cfg, v))

	assert.Equal(t, "localhost:2001", cfg.RemoteServerAddr)
	assert.True(t, cfg.RemoteTLS.Enabled)
	assert.Equal(t, 60*time.Second, cfg.RemoteConnectTimeout)
}

func TestRemoteOptionsNoTLSWithFlags(t *testing.T) {
	v, command := config.Viperize(v1AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=false",
		"--grpc-storage.connection-timeout=60s",
	})
	require.NoError(t, err)
	var cfg Configuration
	require.NoError(t, v1InitFromViper(&cfg, v))

	assert.Equal(t, "localhost:2001", cfg.RemoteServerAddr)
	assert.False(t, cfg.RemoteTLS.Enabled)
	assert.Equal(t, 60*time.Second, cfg.RemoteConnectTimeout)
}

func TestFailedTLSFlags(t *testing.T) {
	v, command := config.Viperize(v1AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.tls.enabled=false",
		"--grpc-storage.tls.cert=blah", // invalid unless tls.enabled=true
	})
	require.NoError(t, err)
	var cfg Configuration
	err = v1InitFromViper(&cfg, v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse gRPC storage TLS options")
}
