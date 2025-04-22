// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"flag"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/internal/config"
)

const defaultOutput = "scripts/utils/config-docs/jaeger-config.schema.json"

var genDocs = GenerateDocs

// Command returns the config-docs command.
func Command(v *viper.Viper) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "config-schema",
		Short: "Generates JSON schema for configuration documentation",
		RunE: func(_ *cobra.Command, _ []string) error {
			return genDocs(output)
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
