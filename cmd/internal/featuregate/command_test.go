// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package featuregate

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/otelcol"
)

func TestAddCommand(t *testing.T) {
	cmd := Command()
	assert.Equal(t, "featuregate [feature-id]", cmd.Use)
}

func TestAddCommand_Panic(t *testing.T) {
	assert.PanicsWithValue(t, "could not find 'featuregate' command", func() {
		command(func() *cobra.Command {
			settings := otelcol.CollectorSettings{}
			otelCmd := otelcol.NewCommand(settings)
			for _, cmd := range otelCmd.Commands() {
				if cmd.Name() == "featuregate" {
					otelCmd.RemoveCommand(cmd)
					break
				}
			}
			return otelCmd
		})
	})
}
