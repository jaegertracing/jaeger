// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"log"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"

	allinone "github.com/jaegertracing/jaeger/cmd/jaeger-v2/internal/all-in-one"
	"github.com/jaegertracing/jaeger/pkg/version"
)

const description = "Jaeger backend v2"

func Command() *cobra.Command {
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	info := component.BuildInfo{
		Command:     "jaeger-v2",
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
	// are present in the args. If not, we pass a custom ConfigProvider, and create the
	// collector manually. Unfortunately, `set`(tings) is passed to NewCommand above
	// by value, otherwise we could've overwritten it in the interceptor and then delegated
	// back to the official RunE.
	otelRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// a bit of a hack to check if '--config' flag was present
		configFlag := cmd.Flag("config").Value.String()
		if configFlag != "" && configFlag != "[]" {
			return otelRunE(cmd, args)
		}
		log.Print("No '--config' flags detected, using default All-in-One configuration with memory storage.")
		log.Print("To customize All-in-One behavior, pass a proper configuration.")
		settings.ConfigProvider = allinone.NewConfigProvider()
		col, err := otelcol.NewCollector(settings)
		if err != nil {
			return err
		}
		return col.Run(cmd.Context())
	}

	cmd.Short = description
	cmd.Long = description

	return cmd
}
