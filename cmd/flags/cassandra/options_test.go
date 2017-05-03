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

package cassandra

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	opts := NewOptions()
	primary := opts.GetPrimary()
	assert.NotEmpty(t, primary.Keyspace)
	assert.NotEmpty(t, primary.Servers)
	assert.Equal(t, 2, primary.ConnectionsPerHost)

	aux := opts.Get("archive")
	assert.Equal(t, primary.Keyspace, aux.Keyspace)
	assert.Equal(t, primary.Servers, aux.Servers)
	assert.Equal(t, primary.ConnectionsPerHost, aux.ConnectionsPerHost)
}

func TestOptionsWithFlags(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ExitOnError)
	opts := NewOptions()
	opts.Bind(flags, "cas", "cas.aux")
	flags.Parse([]string{
		"-cas.keyspace=jaeger",
		"-cas.servers=1.1.1.1,2.2.2.2",
		"-cas.connections-per-host=42",
		"-cas.max-retry-attempts=42",
		"-cas.timeout=42s",
		"-cas.port=4242",
		"-cas.proto-version=3",
		"-cas.socket-keep-alive=42s",
		// a couple overrides
		"-cas.aux.keyspace=jaeger-archive",
		"-cas.aux.servers=3.3.3.3,4.4.4.4",
	})

	primary := opts.GetPrimary()
	assert.Equal(t, "jaeger", primary.Keyspace)
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)

	aux := opts.Get("cas.aux")
	assert.Equal(t, "jaeger-archive", aux.Keyspace)
	assert.Equal(t, []string{"3.3.3.3", "4.4.4.4"}, aux.Servers)
	assert.Equal(t, 42, aux.ConnectionsPerHost)
	assert.Equal(t, 42, aux.MaxRetryAttempts)
	assert.Equal(t, 42*time.Second, aux.Timeout)
	assert.Equal(t, 4242, aux.Port)
	assert.Equal(t, 3, aux.ProtoVersion)
	assert.Equal(t, 42*time.Second, aux.SocketKeepAlive)
}
