// Copyright (c) 2020 The Jaeger Authors.
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

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestCollectorOptionsWithFlags_CheckHostPort(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.http-server.host-port=5678",
		"--collector.grpc-server.host-port=1234",
		"--collector.zipkin.host-port=3456",
	})
	c.InitFromViper(v)

	assert.Equal(t, ":5678", c.CollectorHTTPHostPort)
	assert.Equal(t, ":1234", c.CollectorGRPCHostPort)
	assert.Equal(t, ":3456", c.CollectorZipkinHTTPHostPort)
}

func TestCollectorOptionsWithFlags_CheckFullHostPort(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.http-server.host-port=:5678",
		"--collector.grpc-server.host-port=127.0.0.1:1234",
		"--collector.zipkin.host-port=0.0.0.0:3456",
	})
	c.InitFromViper(v)

	assert.Equal(t, ":5678", c.CollectorHTTPHostPort)
	assert.Equal(t, "127.0.0.1:1234", c.CollectorGRPCHostPort)
	assert.Equal(t, "0.0.0.0:3456", c.CollectorZipkinHTTPHostPort)
}

func TestCollectorOptionsWithFailedHTTPFlags(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	err := command.ParseFlags([]string{
		"--collector.http.tls.enabled=false",
		"--collector.http.tls.cert=blah", // invalid unless tls.enabled
	})
	require.NoError(t, err)
	_, err = c.InitFromViper(v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse HTTP TLS options")
}

func TestCollectorOptionsWithFailedGRPCFlags(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	err := command.ParseFlags([]string{
		"--collector.grpc.tls.enabled=false",
		"--collector.grpc.tls.cert=blah", // invalid unless tls.enabled
	})
	require.NoError(t, err)
	_, err = c.InitFromViper(v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse gRPC TLS options")
}

func TestCollectorOptionsWithFlags_CheckMaxReceiveMessageLength(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.grpc-server.max-message-size=8388608",
	})
	c.InitFromViper(v)

	assert.Equal(t, 8388608, c.CollectorGRPCMaxReceiveMessageLength)
}

func TestCollectorOptionsWithFlags_CheckMaxConnectionAge(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.grpc-server.max-connection-age=5m",
		"--collector.grpc-server.max-connection-age-grace=1m",
	})
	c.InitFromViper(v)

	assert.Equal(t, 5*time.Minute, c.CollectorGRPCMaxConnectionAge)
	assert.Equal(t, time.Minute, c.CollectorGRPCMaxConnectionAgeGrace)
}
