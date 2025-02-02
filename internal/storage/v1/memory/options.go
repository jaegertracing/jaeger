// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"flag"

	"github.com/spf13/viper"
)

const limit = "memory.max-traces"

// Options stores the configuration entries for this storage
type Options struct {
	Configuration Configuration `mapstructure:",squash"`
}

// AddFlags from this storage to the CLI
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(limit, 0, "The maximum amount of traces to store in memory. The default number of traces is unbounded.")
}

// InitFromViper initializes the options struct with values from Viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.Configuration.MaxTraces = v.GetInt(limit)
}
