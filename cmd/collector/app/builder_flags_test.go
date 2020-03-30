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

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/stretchr/testify/assert"
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
