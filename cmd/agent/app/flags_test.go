package app

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBingFlags(t *testing.T) {
	cfg := NewBuilder()
	flags := flag.NewFlagSet("test", flag.ExitOnError)
	cfg.Bind(flags)
	flags.Parse([]string{
		"-collector.host-port=1.2.3.4:555",
		"-discovery.min-peers=42",
		"-http-server.host-port=:8080",
		"-processor.jaeger-binary.server-host-port=:1111",
		"-processor.jaeger-binary.server-max-packet-size=4242",
		"-processor.jaeger-binary.server-queue-size=42",
		"-processor.jaeger-binary.workers=42",
	})
	assert.Equal(t, 3, len(cfg.Processors))
	assert.Equal(t, "1.2.3.4:555", cfg.CollectorHostPort)
	assert.Equal(t, 42, cfg.DiscoveryMinPeers)
	assert.Equal(t, ":8080", cfg.SamplingServer.HostPort)
	assert.Equal(t, ":1111", cfg.Processors[2].Server.HostPort)
	assert.Equal(t, 4242, cfg.Processors[2].Server.MaxPacketSize)
	assert.Equal(t, 42, cfg.Processors[2].Server.QueueSize)
	assert.Equal(t, 42, cfg.Processors[2].Workers)
}
