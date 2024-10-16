// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/spf13/viper"
)

const (
	suffixWorkers                = "workers"
	suffixServerQueueSize        = "server-queue-size"
	suffixServerMaxPacketSize    = "server-max-packet-size"
	suffixServerSocketBufferSize = "server-socket-buffer-size"
	suffixServerHostPort         = "server-host-port"

	processorPrefixFmt = "processor.%s-%s."
	httpServerHostPort = "http-server.host-port"
)

var defaultProcessors = []struct {
	model    Model
	protocol Protocol
	port     int
}{
	{model: "zipkin", protocol: "compact", port: AgentZipkinThriftCompactUDP},
	{model: "jaeger", protocol: "compact", port: AgentJaegerThriftCompactUDP},
	{model: "jaeger", protocol: "binary", port: AgentJaegerThriftBinaryUDP},
}

// AddFlags adds flags for Builder.
func AddFlags(flags *flag.FlagSet) {
	flags.String(
		httpServerHostPort,
		defaultHTTPServerHostPort,
		"host:port of the http server (e.g. for /sampling point and /baggageRestrictions endpoint)")

	for _, p := range defaultProcessors {
		prefix := fmt.Sprintf(processorPrefixFmt, p.model, p.protocol)
		flags.Int(prefix+suffixWorkers, defaultServerWorkers, "how many workers the processor should run")
		flags.Int(prefix+suffixServerQueueSize, defaultQueueSize, "length of the queue for the UDP server")
		flags.Int(prefix+suffixServerMaxPacketSize, defaultMaxPacketSize, "max packet size for the UDP server")
		flags.Int(prefix+suffixServerSocketBufferSize, 0, "socket buffer size for UDP packets in bytes")
		flags.String(prefix+suffixServerHostPort, ":"+strconv.Itoa(p.port), "host:port for the UDP server")
	}
}

// InitFromViper initializes Builder with properties retrieved from Viper.
func (b *Builder) InitFromViper(v *viper.Viper) *Builder {
	for _, processor := range defaultProcessors {
		prefix := fmt.Sprintf(processorPrefixFmt, processor.model, processor.protocol)
		p := &ProcessorConfiguration{Model: processor.model, Protocol: processor.protocol}
		p.Workers = v.GetInt(prefix + suffixWorkers)
		p.Server.QueueSize = v.GetInt(prefix + suffixServerQueueSize)
		p.Server.MaxPacketSize = v.GetInt(prefix + suffixServerMaxPacketSize)
		p.Server.SocketBufferSize = v.GetInt(prefix + suffixServerSocketBufferSize)
		p.Server.HostPort = portNumToHostPort(v.GetString(prefix + suffixServerHostPort))
		b.Processors = append(b.Processors, *p)
	}

	b.HTTPServer.HostPort = portNumToHostPort(v.GetString(httpServerHostPort))
	return b
}

// portNumToHostPort checks if the value is a raw integer port number,
// and converts it to ":{port}" host-port string, otherwise leaves it as is.
func portNumToHostPort(v string) string {
	if _, err := strconv.Atoi(v); err == nil {
		return ":" + v
	}
	return v
}
