// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"embed"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/pkg/version"
)

//go:embed all-in-one.yaml
var yamlAllInOne embed.FS

const description = "Jaeger backend v2"

func Command() *cobra.Command {
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	info := component.BuildInfo{
		Command:     "jaeger",
		Description: description,
		Version:     version.Get().String(),
	}

	settings := otelcol.CollectorSettings{
		BuildInfo: info,
		Factories: factories,
	}

	cmd := otelcol.NewCommand(settings)

	// We want to support running the binary in all-in-one mode without a config file.
	// Since there are no explicit hooks in OTel Collector for that today (as of v0.87),
	// we intercept the official RunE implementation and check if any `--config` flags
	// are present in the args. If not, we create one with all-in-one configuration.
	otelRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		configFlag := cmd.Flag("config")
		if !configFlag.Changed {
			log.Print("No '--config' flags detected, using default All-in-One configuration with memory storage.")
			log.Print("To customize All-in-One behavior, pass a proper configuration.")
			data, err := yamlAllInOne.ReadFile("all-in-one.yaml")
			if err != nil {
				return fmt.Errorf("cannot read embedded all-in-one configuration: %w", err)
			}
			configFlag.Value.Set("yaml:" + string(data))
		}
		return otelRunE(cmd, args)
	}

	cmd.Short = description
	cmd.Long = description

	return cmd
}
