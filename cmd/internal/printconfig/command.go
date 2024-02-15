// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package printconfig

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func printConfigurations(cmd *cobra.Command, v *viper.Viper, includeEmpty bool) {
	keys := v.AllKeys()
	sort.Strings(keys)

	for _, key := range keys {
		value := v.Get(key)
		source := "default"
		if v.IsSet(key) {
			source = "user-assigned"
		}

		if includeEmpty {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %v=%s\n", source, key, value)
		} else if value != nil && value != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %v=%s\n", source, key, value)
		}

	}
}

func Command(v *viper.Viper) *cobra.Command {
	allFlag := true
	rootCmd := &cobra.Command{
		Use:   "print-config",
		Short: "Print names and values of configuration options",
		Long:  "Print names and values of configuration options, distinguishing between default and user-assigned values",
		RunE: func(cmd *cobra.Command, args []string) error {
			printConfigurations(cmd, v, allFlag)
			return nil
		},
	}
	rootCmd.Flags().BoolVarP(&allFlag, "all", "a", false, "Print all configuration options including those with empty values")

	return rootCmd
}
