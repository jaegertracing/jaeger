// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

const (
	remotePrefix             = "grpc-storage"
	remoteServer             = remotePrefix + ".server"
	remoteConnectionTimeout  = remotePrefix + ".connection-timeout"
	defaultConnectionTimeout = time.Duration(5 * time.Second)
)

func tlsFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: remotePrefix,
	}
}

// addFlags adds flags for Options
func addFlags(flagSet *flag.FlagSet) {
	tlsFlagsConfig().AddFlags(flagSet)

	flagSet.String(remoteServer, "", "The remote storage gRPC server address as host:port")
	flagSet.Duration(remoteConnectionTimeout, defaultConnectionTimeout, "The remote storage gRPC server connection timeout")
}

func initFromViper(cfg *Config, v *viper.Viper) error {
	cfg.ClientConfig.Endpoint = v.GetString(remoteServer)
	remoteTLSCfg, err := tlsFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse gRPC storage TLS options: %w", err)
	}
	cfg.ClientConfig.TLSSetting = remoteTLSCfg
	cfg.TimeoutConfig.Timeout = v.GetDuration(remoteConnectionTimeout)
	cfg.Tenancy = tenancy.InitFromViper(v)
	return nil
}
