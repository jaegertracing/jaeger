// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/pkg/config/esrollovercfg"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

var tlsFlagsCfg = tlscfg.ClientFlagsConfig{Prefix: "es"}
var esrolloverCfg = esrollovercfg.EsRolloverFlagConfig{}

const (
	indexPrefix = "index-prefix"
	username    = "es.username"
	password    = "es.password"
	useILM      = "es.use-ilm"
)

// Config holds the global configurations for the es rollover, common to all actions
type Config struct {
	esrollovercfg.RolloverOptions
	IndexPrefix string
	Username    string
	Password    string
	TLSEnabled  bool
	UseILM      bool
	TLSConfig   configtls.ClientConfig
}

// AddFlags adds flags
func AddFlags(flags *flag.FlagSet) {
	esrolloverCfg.AddFlagsForRolloverOptions(flags)
	flags.String(indexPrefix, "", "Index prefix")
	flags.String(username, "", "The username required by storage")
	flags.String(password, "", "The password required by storage")
	flags.Bool(useILM, false, "Use ILM to manage jaeger indices")
	tlsFlagsCfg.AddFlags(flags)
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.RolloverOptions = esrolloverCfg.InitRolloverOptionsFromViper(v)
	c.IndexPrefix = v.GetString(indexPrefix)
	if c.IndexPrefix != "" {
		c.IndexPrefix += "-"
	}
	c.Username = v.GetString(username)
	c.Password = v.GetString(password)
	c.UseILM = v.GetBool(useILM)
	opts, err := tlsFlagsCfg.InitFromViper(v)
	if err != nil {
		panic(err)
	}
	c.TLSConfig = opts.ToOtelClientConfig()
}
