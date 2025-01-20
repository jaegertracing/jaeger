// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mapping"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	options := app.Options{}
	esmappingsCmd := mapping.Command(options)
	esmappingsCmd.AddCommand(version.Command())

	if err := esmappingsCmd.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
