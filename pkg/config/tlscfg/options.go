// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tlscfg

import (
	"time"

	"go.opentelemetry.io/collector/config/configtls"
)

// Options describes the configuration properties for TLS Connections.
type Options struct {
	Enabled        bool          `mapstructure:"enabled"`
	CAPath         string        `mapstructure:"ca"`
	CertPath       string        `mapstructure:"cert"`
	KeyPath        string        `mapstructure:"key"`
	ServerName     string        `mapstructure:"server_name"` // only for client-side TLS config
	ClientCAPath   string        `mapstructure:"client_ca"`   // only for server-side TLS config for client auth
	CipherSuites   []string      `mapstructure:"cipher_suites"`
	MinVersion     string        `mapstructure:"min_version"`
	MaxVersion     string        `mapstructure:"max_version"`
	SkipHostVerify bool          `mapstructure:"skip_host_verify"`
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
}

func (o *Options) ToOtelClientConfig() configtls.ClientConfig {
	return configtls.ClientConfig{
		Insecure:           !o.Enabled,
		InsecureSkipVerify: o.SkipHostVerify,
		ServerName:         o.ServerName,
		Config: configtls.Config{
			CAFile:         o.CAPath,
			CertFile:       o.CertPath,
			KeyFile:        o.KeyPath,
			CipherSuites:   o.CipherSuites,
			MinVersion:     o.MinVersion,
			MaxVersion:     o.MaxVersion,
			ReloadInterval: o.ReloadInterval,

			// when no truststore given, use SystemCertPool
			// https://github.com/jaegertracing/jaeger/issues/6334
			IncludeSystemCACertsPool: o.Enabled && (len(o.CAPath) == 0),
		},
	}
}

// ToOtelServerConfig provides a mapping between from Options to OTEL's TLS Server Configuration.
func (o *Options) ToOtelServerConfig() *configtls.ServerConfig {
	if !o.Enabled {
		return nil
	}

	cfg := &configtls.ServerConfig{
		ClientCAFile: o.ClientCAPath,
		Config: configtls.Config{
			CAFile:         o.CAPath,
			CertFile:       o.CertPath,
			KeyFile:        o.KeyPath,
			CipherSuites:   o.CipherSuites,
			MinVersion:     o.MinVersion,
			MaxVersion:     o.MaxVersion,
			ReloadInterval: o.ReloadInterval,
		},
	}

	if o.ReloadInterval > 0 {
		cfg.ReloadClientCAFile = true
	}

	return cfg
}
