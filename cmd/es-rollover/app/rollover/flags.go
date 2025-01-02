// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package rollover

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/config/esrollovercfg"
)

var esrolloverCfg = esrollovercfg.EsRolloverFlagConfig{}

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	esrollovercfg.RollBackOptions
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	esrolloverCfg.AddFlagsForRollBackOptions(flags)
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	esrolloverCfg.InitRollBackFromViper(v)
}
