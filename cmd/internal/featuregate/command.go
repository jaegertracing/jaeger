// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package featuregate

import (
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"
)

func Command() *cobra.Command {
	var command *cobra.Command
	settings := otelcol.CollectorSettings{}
	otelCmd := otelcol.NewCommand(settings)
	for _, cmd := range otelCmd.Commands() {
		if cmd.Name() == "featuregate" {
			otelCmd.RemoveCommand(cmd)
			command = cmd
		}
	}
	return command
}
