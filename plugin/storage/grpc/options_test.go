package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--grpc-storage-plugin.binary=noop-grpc-plugin",
		"--grpc-storage-plugin.configuration-file=config.json",
	})
	opts.InitFromViper(v)

	assert.Equal(t, opts.Configuration.PluginBinary, "noop-grpc-plugin")
	assert.Equal(t, opts.Configuration.PluginConfigurationFile, "config.json")
}
