// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"flag"
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

const (
	gRPCPrefix        = "reporter.grpc"
	collectorHostPort = gRPCPrefix + ".host-port"
	retryFlag         = gRPCPrefix + ".retry.max"
	defaultMaxRetry   = 3
	discoveryMinPeers = gRPCPrefix + ".discovery.min-peers"
)

var tlsFlagsConfig = tlscfg.ClientFlagsConfig{
	Prefix: gRPCPrefix,
}

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.Uint(retryFlag, defaultMaxRetry, "Sets the maximum number of retries for a call")
	flags.Int(discoveryMinPeers, 3, "Max number of collectors to which the agent will try to connect at any given time")
	flags.String(collectorHostPort, "", "Comma-separated string representing host:port of a static list of collectors to connect to directly")
	tlsFlagsConfig.AddFlags(flags)
}

// InitFromViper initializes Options with properties retrieved from Viper.
func (b *ConnBuilder) InitFromViper(v *viper.Viper) (*ConnBuilder, error) {
	hostPorts := v.GetString(collectorHostPort)
	if hostPorts != "" {
		b.CollectorHostPorts = strings.Split(hostPorts, ",")
	}
	b.MaxRetry = v.GetUint(retryFlag)
	tls, err := tlsFlagsConfig.InitFromViper(v)
	if err != nil {
		return b, fmt.Errorf("failed to process TLS options: %w", err)
	}
	b.TLS = tls
	b.DiscoveryMinPeers = v.GetInt(discoveryMinPeers)
	return b, nil
}
