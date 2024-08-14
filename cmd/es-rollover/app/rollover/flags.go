// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package rollover

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
)

const (
	conditions               = "conditions"
	defaultRollbackCondition = "{\"max_age\": \"2d\"}"
)

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	Conditions string
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.String(conditions, defaultRollbackCondition, "conditions used to rollover to a new write index")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.Conditions = v.GetString(conditions)
}
