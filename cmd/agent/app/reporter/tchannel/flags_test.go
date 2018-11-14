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
	"go.uber.org/zap"
)

func TestBingFlags(t *testing.T) {
	v := viper.New()
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	tests := []struct {
		flags   []string
		builder Builder
	}{
		{flags: []string{
			"--reporter.tchannel.host-port=1.2.3.4:555,1.2.3.4:666",
			"--reporter.tchannel.discovery.min-peers=42",
			"--reporter.tchannel.discovery.conn-check-timeout=85s",
			"--reporter.tchannel.report-timeout=80s",
		}, builder: Builder{ConnCheckTimeout: time.Second * 85, ReportTimeout: time.Second * 80, DiscoveryMinPeers: 42, CollectorHostPorts: []string{"1.2.3.4:555", "1.2.3.4:666"}},
		},
		{flags: []string{
			"--collector.host-port=1.2.3.4:555,1.2.3.4:666",
			"--discovery.min-peers=42",
			"--discovery.conn-check-timeout=85s",
		},
			builder: Builder{ConnCheckTimeout: time.Second * 85, ReportTimeout: defaultReportTimeout, DiscoveryMinPeers: 42, CollectorHostPorts: []string{"1.2.3.4:555", "1.2.3.4:666"}},
		},
		{flags: []string{
			"--collector.host-port=1.2.3.4:555,1.2.3.4:666",
			"--discovery.min-peers=42",
			"--discovery.conn-check-timeout=85s",
			"--reporter.tchannel.host-port=1.2.3.4:5556,1.2.3.4:6667",
			"--reporter.tchannel.discovery.min-peers=43",
			"--reporter.tchannel.discovery.conn-check-timeout=86s",
		},
			builder: Builder{ConnCheckTimeout: time.Second * 86, ReportTimeout: defaultReportTimeout, DiscoveryMinPeers: 43, CollectorHostPorts: []string{"1.2.3.4:5556", "1.2.3.4:6667"}},
		},
	}
	for _, test := range tests {
		err := command.ParseFlags(test.flags)
		require.NoError(t, err)
		b := Builder{}
		b.InitFromViper(v, zap.NewNop())
		assert.Equal(t, test.builder, b)
	}
}
