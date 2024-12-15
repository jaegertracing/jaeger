// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestBindFlags(t *testing.T) {
	tests := []struct {
		cOpts    []string
		expected *ConnBuilder
	}{
		{
			cOpts:    []string{"--reporter.grpc.host-port=localhost:1111", "--reporter.grpc.retry.max=15"},
			expected: &ConnBuilder{CollectorHostPorts: []string{"localhost:1111"}, MaxRetry: 15, DiscoveryMinPeers: 3},
		},
		{
			cOpts:    []string{"--reporter.grpc.host-port=localhost:1111,localhost:2222"},
			expected: &ConnBuilder{CollectorHostPorts: []string{"localhost:1111", "localhost:2222"}, MaxRetry: defaultMaxRetry, DiscoveryMinPeers: 3},
		},
		{
			cOpts:    []string{"--reporter.grpc.host-port=localhost:1111,localhost:2222", "--reporter.grpc.discovery.min-peers=5"},
			expected: &ConnBuilder{CollectorHostPorts: []string{"localhost:1111", "localhost:2222"}, MaxRetry: defaultMaxRetry, DiscoveryMinPeers: 5},
		},
		{
			cOpts: []string{"--reporter.grpc.tls.enabled=true"},
			expected: &ConnBuilder{
				TLS: &configtls.ClientConfig{
					Config: configtls.Config{
						IncludeSystemCACertsPool: true,
					},
				},
				MaxRetry: defaultMaxRetry, DiscoveryMinPeers: 3,
			},
		},
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
		b, err := new(ConnBuilder).InitFromViper(v)
		require.NoError(t, err)
		assert.Equal(t, test.expected, b)
	}
}

func TestBindTLSFlagFailure(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	err := command.ParseFlags([]string{
		"--reporter.grpc.tls.enabled=false",
		"--reporter.grpc.tls.cert=blah", // invalid unless tls.enabled
	})
	require.NoError(t, err)
	_, err = new(ConnBuilder).InitFromViper(v)
	assert.ErrorContains(t, err, "failed to process TLS options")
}
