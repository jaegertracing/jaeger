// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"
)

const (
	defaultJSONOutput     = "docs/jaeger-v2-config.schema.json"
	defaultMarkdownOutput = "docs/jaeger-v2-config.md"
)

// generateFunc is the schema generator used by the CLI. It is exposed as a
// package variable so tests can inject a deterministic implementation.
var generateFunc = Generate

// Command returns the config-schema cobra subcommand that writes JSON schema
// and/or Markdown documentation for Jaeger v2 component configurations.
func Command(factories otelcol.Factories) *cobra.Command {
	var (
		output string
		format string
	)

	cmd := &cobra.Command{
		Use:   "config-schema",
		Short: "Generate JSON schema or Markdown docs for Jaeger v2 config structs",
		Long: strings.TrimSpace(`
The config-schema command auto-generates documentation directly from the Go
source code of the Jaeger v2 configuration structs. It reduces drift between
code and documentation by deriving property names from mapstructure tags,
descriptions from doc comments, defaults from the runtime default values, and
required flags from the absence of omitempty/squash.`),
		RunE: func(_ *cobra.Command, _ []string) error {
			switch format {
			case "json", "markdown":
			default:
				return fmt.Errorf("unsupported format %q (use json or markdown)", format)
			}

			schema, err := generateFunc(factories)
			if err != nil {
				return fmt.Errorf("generating schema: %w", err)
			}

			if format == "json" {
				return writeJSON(schema, output)
			}
			return writeMarkdown(schema, output)
		},
	}

	cmd.Flags().StringVar(&output, "output", defaultJSONOutput, "output file path")
	cmd.Flags().StringVar(&format, "format", "json", "output format: json or markdown")
	return cmd
}

func writeJSON(schema *Schema, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// writeMarkdown renders the schema as Markdown documentation.
func writeMarkdown(schema *Schema, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return renderMarkdown(schema, path)
}
