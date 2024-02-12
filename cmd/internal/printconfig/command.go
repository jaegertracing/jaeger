// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
