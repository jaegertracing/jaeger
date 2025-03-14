// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Viperize creates new Viper and command add passes flags to command
// Viper is initialized with flags from command and configured to accept flags as environmental variables.
// Characters `.-` in environmental variables are changed to `_`
func Viperize(inits ...func(*flag.FlagSet)) (*viper.Viper, *cobra.Command) {
	return AddFlags(viper.New(), &cobra.Command{}, inits...)
}

// AddFlags adds flags to command and viper and configures
func AddFlags(v *viper.Viper, command *cobra.Command, inits ...func(*flag.FlagSet)) (*viper.Viper, *cobra.Command) {
	flagSet := new(flag.FlagSet)
	for i := range inits {
		inits[i](flagSet)
	}
	command.Flags().AddGoFlagSet(flagSet)
	command.Flags().AddFlagSet(pflag.CommandLine)
	configureViper(v)
	v.BindPFlags(command.Flags())
	return v, command
}

func configureViper(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
}
