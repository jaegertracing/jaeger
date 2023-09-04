// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app/renderer"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	logger, _ := zap.NewDevelopment()
	options := app.Options{}
	command := &cobra.Command{
		Use:   "jaeger-esmapping-generator",
		Short: "Jaeger esmapping-generator prints rendered mappings as string",
		Long:  `Jaeger esmapping-generator renders passed templates with provided values and prints rendered output to stdout`,
		Run: func(cmd *cobra.Command, args []string) {
			if !renderer.IsValidOption(options.Mapping) {
				logger.Fatal("please pass either 'jaeger-service' or 'jaeger-span' as argument")
			}

			parsedMapping, err := renderer.GetMappingAsString(es.TextTemplateBuilder{}, &options)
			if err != nil {
				logger.Fatal(err.Error())
			}
			print(parsedMapping)
		},
	}

	options.AddFlags(command)

	command.AddCommand(version.Command())

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
