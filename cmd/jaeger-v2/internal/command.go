// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"log"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/pkg/version"
)

const description = "Jaeger backend v2"

func Command() *cobra.Command {
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	versionInfo := version.Get()

	info := component.BuildInfo{
		Command:     "jaeger-v2",
		Description: description,
		Version:     versionInfo.GitVersion,
	}

	cmd := otelcol.NewCommand(
		otelcol.CollectorSettings{
			BuildInfo: info,
			Factories: factories,
		},
	)

	cmd.Short = description
	cmd.Long = description

	return cmd
}
