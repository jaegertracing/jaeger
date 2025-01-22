// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mapping

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/generator"
)

func Command() *cobra.Command {
	options := app.Options{}
	command := &cobra.Command{
		Use:   "elasticsearch-mappings",
		Short: "Jaeger esmapping-generator prints rendered mappings as string",
		Long:  "Jaeger esmapping-generator renders passed templates with provided values and prints rendered output to stdout",
		Run: func(_ *cobra.Command, _ /* args */ []string) {
			result, err := generator.GenerateMappings(options)
			if err != nil {
				log.Fatalf("Error generating mappings: %v", err)
			}
			fmt.Println(result)
		},
	}
	options.AddFlags(command)

	return command
}
