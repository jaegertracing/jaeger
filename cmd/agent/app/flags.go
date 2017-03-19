package app

import "flag"

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
	flags.StringVar(
		&b.CollectorHostPort,
		"collector.host-port",
		"",
		"host:port of a single collector to connect to directly (e.g. when not using service discovery)")
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
