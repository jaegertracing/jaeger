// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package printconfig

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func printDivider(cmd *cobra.Command, n int) {
	fmt.Fprint(cmd.OutOrStdout(), strings.Repeat("-", n), "\n")
}

func printConfigurations(cmd *cobra.Command, v *viper.Viper, includeEmpty bool) {
	keys := v.AllKeys()
	sort.Strings(keys)

	maxKeyLength, maxValueLength := len("Configuration Option Name"), len("Value")
	maxSourceLength := len("user-assigned")
	for _, key := range keys {
		value := v.GetString(key)
		if len(key) > maxKeyLength {
			maxKeyLength = len(key)
		}
		if len(value) > maxValueLength {
			maxValueLength = len(value)
		}
	}
	maxRowLength := maxKeyLength + maxValueLength + maxSourceLength + 6

	printDivider(cmd, maxRowLength)
	fmt.Fprintf(cmd.OutOrStdout(),
		"| %-*s %-*s %-*s |\n",
		maxKeyLength, "Configuration Option Name",
		maxValueLength, "Value",
		maxSourceLength, "Source")
	printDivider(cmd, maxRowLength)

	for _, key := range keys {
		value := v.GetString(key)
		source := "default"
		if v.IsSet(key) {
			source = "user-assigned"
		}

		if includeEmpty || value != "" {
			fmt.Fprintf(cmd.OutOrStdout(),
				"| %-*s %-*s %-*s |\n",
				maxKeyLength, key,
				maxValueLength, value,
				maxSourceLength, source)
		}
	}
	printDivider(cmd, maxRowLength)
}

func Command(v *viper.Viper) *cobra.Command {
	allFlag := true
	cmd := &cobra.Command{
		Use:   "print-config",
		Short: "Print names and values of configuration options",
		Long:  "Print names and values of configuration options, distinguishing between default and user-assigned values",
		RunE: func(cmd *cobra.Command, args []string) error {
			printConfigurations(cmd, v, allFlag)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&allFlag, "all", "a", false, "Print all configuration options including those with empty values")

	return cmd
}
