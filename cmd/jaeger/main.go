// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/features"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	v := viper.New()
	command := internal.Command()
	command.AddCommand(version.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(featuresCommand())

	config.AddFlags(
		v,
		command,
	)

	if err := command.Execute(); err != nil {
		log.Fatal(err)
	}
}

func featuresCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "features",
		Short: "Displays the list of supported features",
		Long:  "The 'features' command shows all supported features in the current build of Jaeger.",
		Run: func(_ *cobra.Command, _ []string) {
			features.DisplayFeatures()
		},
	}
}
