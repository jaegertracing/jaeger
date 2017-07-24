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

package app

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBingFlags(t *testing.T) {
	v := viper.New()
	b := &Builder{}
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--collector.host-port=1.2.3.4:555,1.2.3.4:666",
		"--discovery.min-peers=42",
		"--http-server.host-port=:8080",
		"--processor.jaeger-binary.server-host-port=:1111",
		"--processor.jaeger-binary.server-max-packet-size=4242",
		"--processor.jaeger-binary.server-queue-size=42",
		"--processor.jaeger-binary.workers=42",
	})
	require.NoError(t, err)

	b.InitFromViper(v)
	assert.Equal(t, 3, len(b.Processors))
	assert.Equal(t, []string{"1.2.3.4:555", "1.2.3.4:666"}, b.CollectorHostPorts)
	assert.Equal(t, 42, b.DiscoveryMinPeers)
	assert.Equal(t, ":8080", b.HTTPServer.HostPort)
	assert.Equal(t, ":1111", b.Processors[2].Server.HostPort)
	assert.Equal(t, 4242, b.Processors[2].Server.MaxPacketSize)
	assert.Equal(t, 42, b.Processors[2].Server.QueueSize)
	assert.Equal(t, 42, b.Processors[2].Workers)
}
