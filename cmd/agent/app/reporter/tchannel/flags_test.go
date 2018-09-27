// Copyright (c) 2018 The Jaeger Authors.
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

package tchannel

import (
	"flag"
	"testing"
	"time"

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
		"--reporter.tchannel.collector.host-port=1.2.3.4:555,1.2.3.4:666",
		"--reporter.tchannel.discovery.min-peers=42",
		"--reporter.tchannel.discovery.conn-check-timeout=85s",
	})
	require.NoError(t, err)

	b.InitFromViper(v)
	assert.Equal(t, 42, b.DiscoveryMinPeers)
	assert.Equal(t, []string{"1.2.3.4:555", "1.2.3.4:666"}, b.CollectorHostPorts)
	assert.Equal(t, time.Second*85, b.ConnCheckTimeout)
}
