// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package jaegercli exposes Jaeger's root CLI command for reuse in custom
// OpenTelemetry Collector Builder (OCB) distributions, so they retain the
// embedded all-in-one default config and Jaeger-specific subcommands.
package jaegercli

import (
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

// NewCommand returns Jaeger's root cobra command wired with the given component
// factories. It is a thin redirection to internal.NewCommand.
func NewCommand(factories otelcol.Factories) *cobra.Command {
	return internal.NewCommand(factories)
}
