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

package grpc

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindFlags(t *testing.T) {
	tests := []struct {
		cOpts    []string
		expected *ConnBuilder
	}{
		{cOpts: []string{"--reporter.grpc.host-port=localhost:1111", "--reporter.grpc.retry.max=15"},
			expected: &ConnBuilder{CollectorHostPorts: []string{"localhost:1111"}, MaxRetry: 15, DiscoveryMinPeers: 3}},
		{cOpts: []string{"--reporter.grpc.host-port=localhost:1111,localhost:2222"},
			expected: &ConnBuilder{CollectorHostPorts: []string{"localhost:1111", "localhost:2222"}, MaxRetry: defaultMaxRetry, DiscoveryMinPeers: 3}},
		{cOpts: []string{"--reporter.grpc.host-port=localhost:1111,localhost:2222", "--reporter.grpc.discovery.min-peers=5"},
			expected: &ConnBuilder{CollectorHostPorts: []string{"localhost:1111", "localhost:2222"}, MaxRetry: defaultMaxRetry, DiscoveryMinPeers: 5}},
	}
	for _, test := range tests {
		v := viper.New()
		command := cobra.Command{}
		flags := &flag.FlagSet{}
		AddFlags(flags)
		command.PersistentFlags().AddGoFlagSet(flags)
		v.BindPFlags(command.PersistentFlags())

		err := command.ParseFlags(test.cOpts)
		require.NoError(t, err)
		b := new(ConnBuilder).InitFromViper(v)
		assert.Equal(t, test.expected, b)
	}
}
