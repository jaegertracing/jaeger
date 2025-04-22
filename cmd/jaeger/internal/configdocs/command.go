// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configdocs

import (
	"flag"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/internal/config"
)

// Command returns the config-docs command.
func Command(v *viper.Viper) *cobra.Command {
	const defaultOutput = "scripts/utils/config-docs/jaeger-config.schema.json"
	var output string
	cmd := &cobra.Command{
		Use:   "config-docs",
		Short: "Generates JSON schema for configuration documentation",
		RunE: func(cmd *cobra.Command, args []string) error {
			return GenerateDocs(output)
		},
	}
	config.AddFlags(
		v,
		cmd,
		func(flagSet *flag.FlagSet) {
			flagSet.StringVar(&output, "output", defaultOutput, "Output file")
		},
	)
	return cmd
}
