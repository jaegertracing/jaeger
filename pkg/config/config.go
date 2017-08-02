// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
	command := &cobra.Command{}
	flagSet := new(flag.FlagSet)
	for i := range inits {
		inits[i](flagSet)
	}
	command.PersistentFlags().AddGoFlagSet(flagSet)

	v := viper.New()
	configureViper(v)
	v.BindPFlags(command.PersistentFlags())
	return v, command
}

// AddFlags adds flags to command and viper and configures
func AddFlags(v *viper.Viper, command *cobra.Command, inits ...func(*flag.FlagSet)) (*viper.Viper, *cobra.Command) {
	flagSet := new(flag.FlagSet)
	for i := range inits {
		inits[i](flagSet)
	}
	command.PersistentFlags().AddGoFlagSet(flagSet)

	configureViper(v)
	v.BindPFlags(command.PersistentFlags())
	return v, command
}

func configureViper(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
}
