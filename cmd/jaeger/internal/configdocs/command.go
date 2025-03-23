// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configdocs

import (
	"github.com/spf13/cobra"
)

// Command returns the config-docs command.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "config-docs",
		Short: "Extracts and generates configuration documentation from structs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return GenerateDocs()
		},
	}
}
