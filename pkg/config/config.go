// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"flag"
	"strings"

	"github.com/spf13/cobra"
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

	configureViper(v)
	v.BindPFlags(command.Flags())
	return v, command
}

func configureViper(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
}
