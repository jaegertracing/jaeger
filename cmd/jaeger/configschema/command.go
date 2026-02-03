// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/expvar"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotestorage"
)

// Command creates the generate-config-schema command
func Command() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "generate-config-schema",
		Short: "Generate configuration schema documentation",
		Long: `Generates documentation for Jaeger-v2 configuration structures.
Extracts field information, comments, and default values from Go structs.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runGenerateSchema(outputFile)
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "",
		"Output file path (default: stdout)")

	return cmd
}

func runGenerateSchema(outputFile string) error {
	// Collect all config objects
	configs := collectConfigs()

	// Extract metadata using reflection + AST
	collection, err := extractConfigInfoWithComments(configs)
	if err != nil {
		return fmt.Errorf("failed to extract config info: %w", err)
	}

	// Format and output
	var writer io.Writer
	if outputFile != "" {
		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()
		writer = file
	} else {
		writer = os.Stdout
	}

	printer := NewPrinter(FormatJSONPretty, writer)
	return printer.Print(collection)
}

// collectConfigs returns all configuration objects to document
func collectConfigs() []any {
	return []any{
		// Jaeger Query Service - UI and query endpoints
		jaegerquery.NewFactory().CreateDefaultConfig(),

		// Jaeger Storage - trace and metric storage backends
		jaegerstorage.NewFactory().CreateDefaultConfig(),

		// Remote Sampling - dynamic sampling strategy
		remotesampling.NewFactory().CreateDefaultConfig(),

		// Remote Storage - remote storage for e2e tests
		remotestorage.NewFactory().CreateDefaultConfig(),

		// Expvar - metrics exposure via expvar
		expvar.NewFactory().CreateDefaultConfig(),

		// Jaeger MCP - Model Context Protocol server
		jaegermcp.NewFactory().CreateDefaultConfig(),
	}
}
