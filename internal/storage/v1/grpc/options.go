// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

const (
	remotePrefix             = "grpc-storage"
	archiveRemotePrefix      = "grpc-storage-archive"
	remoteServer             = ".server"
	remoteConnectionTimeout  = ".connection-timeout"
	enabled                  = ".enabled"
	defaultConnectionTimeout = time.Duration(5 * time.Second)
)

type options struct {
	Config
	namespace string
}

func newOptions(namespace string) *options {
	options := &options{
		Config:    DefaultConfig(),
		namespace: namespace,
	}
	return options
}

func (opts *options) tlsFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: opts.namespace,
	}
}

// addFlags adds flags for Options
func (opts *options) addFlags(flagSet *flag.FlagSet) {
	opts.tlsFlagsConfig().AddFlags(flagSet)

	flagSet.String(opts.namespace+remoteServer, "", "The remote storage gRPC server address as host:port")
	flagSet.Duration(opts.namespace+remoteConnectionTimeout, defaultConnectionTimeout, "The remote storage gRPC server connection timeout")
	if opts.namespace == archiveRemotePrefix {
		flagSet.Bool(
			opts.namespace+enabled,
			false,
			"Enable extra storage")
	}
}

func (opts *options) initFromViper(cfg *Config, v *viper.Viper) error {
	cfg.ClientConfig.Endpoint = v.GetString(opts.namespace + remoteServer)
	remoteTLSCfg, err := opts.tlsFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse gRPC storage TLS options: %w", err)
	}
	cfg.ClientConfig.TLSSetting = remoteTLSCfg
	cfg.TimeoutConfig.Timeout = v.GetDuration(opts.namespace + remoteConnectionTimeout)
	cfg.Tenancy = tenancy.InitFromViper(v)
	if opts.namespace == archiveRemotePrefix {
		cfg.enabled = v.GetBool(opts.namespace + enabled)
	}
	return nil
}
