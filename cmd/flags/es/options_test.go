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

package es

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/pkg/config"
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
		"--es.server-urls=1.1.1.1,2.2.2.2",
		"--es.username=hello",
		"--es.password=world",
		"--es.sniffer=true",
		"--es.max-span-age=48h",
		"--es.num-shards=20",
		"--es.num-replicas=10",
		// a couple overrides
		"--es.aux.server-urls=3.3.3.3,4.4.4.4",
		"--es.aux.max-span-age=24h",
		"--es.aux.num-replicas=10",
	})
	opts.InitFromViper(v)

	primary := opts.GetPrimary()
	assert.Equal(t, "hello", primary.Username)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	assert.Equal(t, 48*time.Hour, primary.MaxSpanAge)
	assert.True(t, primary.Sniffer)

	aux := opts.Get("es.aux")
	assert.Equal(t, []string{"3.3.3.3", "4.4.4.4"}, aux.Servers)
	assert.Equal(t, "hello", aux.Username)
	assert.Equal(t, "world", aux.Password)
	assert.Equal(t, int64(20), aux.NumShards)
	assert.Equal(t, int64(10), aux.NumReplicas)
	assert.Equal(t, 24*time.Hour, aux.MaxSpanAge)
	assert.True(t, aux.Sniffer)

}
