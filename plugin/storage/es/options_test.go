// Copyright (c) 2019 The Jaeger Authors.
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

package es

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptions(t *testing.T) {
	opts := NewOptions("foo")
	primary := opts.GetPrimary()
	assert.Empty(t, primary.Username)
	assert.Empty(t, primary.Password)
	assert.NotEmpty(t, primary.Servers)
	assert.Equal(t, int64(5), primary.NumShards)
	assert.Equal(t, int64(1), primary.NumReplicas)
	assert.Equal(t, 72*time.Hour, primary.MaxSpanAge)
	assert.False(t, primary.Sniffer)

	aux := opts.Get("archive")
	assert.Equal(t, primary.Username, aux.Username)
	assert.Equal(t, primary.Password, aux.Password)
	assert.Equal(t, primary.Servers, aux.Servers)
}

func TestOptionsWithFlags(t *testing.T) {
	opts := NewOptions("es", "es.aux")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--es.server-urls=1.1.1.1, 2.2.2.2",
		"--es.username=hello",
		"--es.password=world",
		"--es.token-file=/foo/bar",
		"--es.sniffer=true",
		"--es.max-span-age=48h",
		"--es.num-shards=20",
		"--es.num-replicas=10",
		// a couple overrides
		"--es.aux.server-urls=3.3.3.3, 4.4.4.4",
		"--es.aux.max-span-age=24h",
		"--es.aux.num-replicas=10",
		"--es.tls.enabled=true",
		"--es.tls.skip-host-verify=true",
	})
	opts.InitFromViper(v)

	primary := opts.GetPrimary()
	assert.Equal(t, "hello", primary.Username)
	assert.Equal(t, "/foo/bar", primary.TokenFilePath)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	assert.Equal(t, 48*time.Hour, primary.MaxSpanAge)
	assert.True(t, primary.Sniffer)
	assert.Equal(t, true, primary.TLS.Enabled)
	assert.Equal(t, true, primary.TLS.SkipHostVerify)

	aux := opts.Get("es.aux")
	assert.Equal(t, []string{"3.3.3.3", "4.4.4.4"}, aux.Servers)
	assert.Equal(t, "hello", aux.Username)
	assert.Equal(t, "world", aux.Password)
	assert.Equal(t, int64(20), aux.NumShards)
	assert.Equal(t, int64(10), aux.NumReplicas)
	assert.Equal(t, 24*time.Hour, aux.MaxSpanAge)
	assert.True(t, aux.Sniffer)

}
