// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

const (
	indexPrefix        = "index-prefix"
	archive            = "archive"
	rollover           = "rollover"
	timeout            = "timeout"
	ignoreUnavailable  = "ignore-unavailable"
	indexDateSeparator = "index-date-separator"
	username           = "es.username"
	password           = "es.password"
)

var tlsFlagsCfg = tlscfg.ClientFlagsConfig{Prefix: "es"}

// Config holds configuration for index cleaner binary.
type Config struct {
	IndexPrefix              string
	Archive                  bool
	Rollover                 bool
	MasterNodeTimeoutSeconds int
	IgnoreUnavailableIndex   bool
	IndexDateSeparator       string
	Username                 string
	Password                 string
	TLSEnabled               bool
	TLSConfig                configtls.ClientConfig
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.String(indexPrefix, "", "Index prefix")
	flags.Bool(archive, false, "Whether to remove archive indices. It works only for rollover")
	flags.Bool(rollover, false, "Whether to remove indices created by rollover")
	flags.Int(timeout, 120, "Number of seconds to wait for master node response")
	flags.Bool(ignoreUnavailable, true, "If false, returns an error when index is missing")
	flags.String(indexDateSeparator, "-", "Index date separator")
	flags.String(username, "", "The username required by storage")
	flags.String(password, "", "The password required by storage")
	tlsFlagsCfg.AddFlags(flags)
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.IndexPrefix = v.GetString(indexPrefix)
	if c.IndexPrefix != "" {
		c.IndexPrefix += "-"
	}

	c.Archive = v.GetBool(archive)
	c.Rollover = v.GetBool(rollover)
	c.MasterNodeTimeoutSeconds = v.GetInt(timeout)
	c.IgnoreUnavailableIndex = v.GetBool(ignoreUnavailable)
	c.IndexDateSeparator = v.GetString(indexDateSeparator)
	c.Username = v.GetString(username)
	c.Password = v.GetString(password)
	tlsCfg, err := tlsFlagsCfg.InitFromViper(v)
	if err != nil {
		panic(err)
	}
	c.TLSConfig = tlsCfg
}
