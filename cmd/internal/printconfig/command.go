// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
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

package printconfig

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
		Long:  `Iterates through the overall configuration used by Jaeger and prints them out`,
		RunE: func(cmd *cobra.Command, args []string) error {

			keys := v.AllKeys()
			sort.Strings(keys)
			for _, key := range keys {
				value := v.Get(key)
				str := fmt.Sprintf("%s=%v\n", key, value)
				fmt.Fprint(cmd.OutOrStdout(), str)
			}
			return nil
		},
	}
	return c
}
