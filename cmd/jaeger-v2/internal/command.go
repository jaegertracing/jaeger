// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
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

	versionInfo := version.Get()

	info := component.BuildInfo{
		Command:     "jaeger-v2",
		Description: description,
		Version:     versionInfo.GitVersion,
	}

	cmd := otelcol.NewCommand(
		otelcol.CollectorSettings{
			BuildInfo:      info,
			Factories:      factories,
			ConfigProvider: allinone.NewConfigProvider(),
		},
	)

	cmd.Short = description
	cmd.Long = description

	return cmd
}

type AllInOneConfigProvider struct{}

var _ otelcol.ConfigProvider = (*AllInOneConfigProvider)(nil)

func Get(ctx context.Context, factories otelcol.Factories) (*otelcol.Config, error) {
	return nil, nil
}
