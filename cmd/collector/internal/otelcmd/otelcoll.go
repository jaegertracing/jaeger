// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelcmd

import (
	"log"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

func Command() *cobra.Command {
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	info := component.BuildInfo{
		Command:     "otel",
		Description: "OpenTelemtery Collector compatible mode",
		Version:     "2.0.0",
	}

	cmd := otelcol.NewCommand(
		otelcol.CollectorSettings{
			BuildInfo: info,
			Factories: factories,
		},
	)

	cmd.Short = "Run in OpenTelemtery Collector compatible mode."
	cmd.Long = "Run in OpenTelemtery Collector compatible mode."

	return cmd
}
