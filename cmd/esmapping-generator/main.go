// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app/renderer"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	logger, _ := zap.NewDevelopment()
	options := app.Options{}
	command := &cobra.Command{
		Use:   "jaeger-esmapping-generator",
		Short: "Jaeger esmapping-generator prints rendered mappings as string",
		Long:  `Jaeger esmapping-generator renders passed templates with provided values and prints rendered output to stdout`,
		Run: func(_ *cobra.Command, _ /* args */ []string) {
			if err := renderer.GenerateESMappings(logger, options); err != nil {
				logger.Fatal("failed to generate mappings: " + err.Error())
			}
		},
	}

	command.Flags().StringVar(&options.Mapping, "mapping", "jaeger-span", "Mapping type to use, e.g. 'jaeger-service' or 'jaeger-span'")

	command.AddCommand(version.Command())

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
