// Copyright (c) 2018 The Jaeger Authors.
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
package print_config

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Command(v *viper.Viper) *cobra.Command {
	c := &cobra.Command{
		Use:   "print-config",
		Short: "Print configurations",
		Long:  `Iterates through the environment variables and prints them out`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys := v.AllKeys()
			sort.Strings(keys)

			for _, env := range keys {
				value := v.Get(env)
				fmt.Printf("%s=%v\n", env, value)
			}
			return nil
		},
	}
	return c
}
