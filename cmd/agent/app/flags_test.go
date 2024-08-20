// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindFlags(t *testing.T) {
	v := viper.New()
	b := &Builder{}
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--http-server.host-port=:8080",
		"--processor.jaeger-binary.server-host-port=:1111",
		"--processor.jaeger-binary.server-max-packet-size=4242",
		"--processor.jaeger-binary.server-queue-size=42",
		"--processor.jaeger-binary.workers=42",
	})
	require.NoError(t, err)

	b.InitFromViper(v)
	assert.Len(t, b.Processors, 3)
	assert.Equal(t, ":8080", b.HTTPServer.HostPort)
	assert.Equal(t, ":1111", b.Processors[2].Server.HostPort)
	assert.Equal(t, 4242, b.Processors[2].Server.MaxPacketSize)
	assert.Equal(t, 42, b.Processors[2].Server.QueueSize)
	assert.Equal(t, 42, b.Processors[2].Workers)
}
