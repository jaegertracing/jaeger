// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package lookback

import (
	"flag"

	"github.com/spf13/viper"

	app "github.com/jaegertracing/jaeger/pkg/esrollover"
)

const (
	unit             = "unit"
	unitCount        = "unit-count"
	defaultUnit      = "days"
	defaultUnitCount = 1
)

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	Unit      string
	UnitCount int
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.String(unit, defaultUnit, "used with lookback to remove indices from read alias e.g, days, weeks, months, years")
	flags.Int(unitCount, defaultUnitCount, "count of UNITs")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.Unit = v.GetString(unit)
	c.UnitCount = v.GetInt(unitCount)
}
