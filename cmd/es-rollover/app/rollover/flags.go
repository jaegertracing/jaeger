// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package rollover

import (
	"flag"
	"github.com/jaegertracing/jaeger/pkg/esrollover"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
)

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	esrollover.RollBackConditions
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	cfg := esrollover.EsRolloverFlagConfig{}
	cfg.AddFlagsForRollBack(flags)
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	cfg := esrollover.EsRolloverFlagConfig{}
	c.RollBackConditions = *cfg.InitFromViperForRollBack(v)
}
