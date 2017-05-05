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
	"errors"
	"flag"
	"fmt"
	"strings"
)

// Bind binds the agent builder to command line options
func (b *Builder) Bind(flags *flag.FlagSet) {
	for i := range b.Processors {
		p := &b.Processors[i]
		name := "processor." + string(p.Model) + "-" + string(p.Protocol) + "."
		flags.IntVar(
			&p.Workers,
			name+"workers",
			p.Workers,
			"how many workers the processor should run")
		flags.IntVar(
			&p.Server.QueueSize,
			name+"server-queue-size",
			p.Server.QueueSize,
			"length of the queue for the UDP server")
		flags.IntVar(
			&p.Server.MaxPacketSize,
			name+"server-max-packet-size",
			p.Server.MaxPacketSize,
			"max packet size for the UDP server")
		flags.StringVar(
			&p.Server.HostPort,
			name+"server-host-port",
			p.Server.HostPort,
			"host:port for the UDP server")
	}
	flags.Var(
		&stringSliceFlag{slice: &b.CollectorHostPorts},
		"collector.host-port",
		"comma-separated string representing host:ports of a static list of collectors to connect to directly (e.g. when not using service discovery)")
	flags.StringVar(
		&b.SamplingServer.HostPort,
		"http-server.host-port",
		b.SamplingServer.HostPort,
		"host:port of the http server (e.g. for /sampling point)")
	flags.IntVar(
		&b.DiscoveryMinPeers,
		"discovery.min-peers",
		3,
		"if using service discovery, the min number of connections to maintain to the backend")
}

type stringSliceFlag struct {
	slice *[]string
}

// String formats the flag's value, part of the flag.Value interface.
func (c *stringSliceFlag) String() string {
	return fmt.Sprint(c.slice)
}

// Set sets the flag value, part of the flag.Value interface.
func (c *stringSliceFlag) Set(value string) error {
	if len(*(c.slice)) > 0 {
		return errors.New("comma-separated flag already set")
	}

	hostPorts := strings.Split(value, ",")
	*(c.slice) = append(*(c.slice), hostPorts...)

	return nil
}
