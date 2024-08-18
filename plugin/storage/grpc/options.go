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

// AddFlags adds flags for Options
func v1AddFlags(flagSet *flag.FlagSet) {
	tlsFlagsConfig().AddFlags(flagSet)

	flagSet.String(remoteServer, "", "The remote storage gRPC server address as host:port")
	flagSet.Duration(remoteConnectionTimeout, defaultConnectionTimeout, "The remote storage gRPC server connection timeout")
}

func v1InitFromViper(cfg *Configuration, v *viper.Viper) error {
	cfg.RemoteServerAddr = v.GetString(remoteServer)
	var err error
	cfg.RemoteTLS, err = tlsFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse gRPC storage TLS options: %w", err)
	}
	cfg.RemoteConnectTimeout = v.GetDuration(remoteConnectionTimeout)
	cfg.TenancyOpts = tenancy.InitFromViper(v)
	return nil
}
