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
		"--http-server.host-port=:8080",
		"--processor.jaeger-binary.server-host-port=:1111",
		"--processor.jaeger-binary.server-max-packet-size=4242",
		"--processor.jaeger-binary.server-queue-size=42",
		"--processor.jaeger-binary.workers=42",
	})
	require.NoError(t, err)

	b.InitFromViper(v)
	assert.Equal(t, 3, len(b.Processors))
	assert.Equal(t, ":8080", b.HTTPServer.HostPort)
	assert.Equal(t, ":1111", b.Processors[2].Server.HostPort)
	assert.Equal(t, 4242, b.Processors[2].Server.MaxPacketSize)
	assert.Equal(t, 42, b.Processors[2].Server.QueueSize)
	assert.Equal(t, 42, b.Processors[2].Workers)
}
