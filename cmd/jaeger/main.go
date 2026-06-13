// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
	"github.com/jaegertracing/jaeger/internal/version"
)

func main() {
	v := viper.New()
	command := internal.Command()
	command.AddCommand(version.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(mappings.Command())
	config.AddFlags(
		v,
		command,
	)

	if err := command.Execute(); err != nil {
		log.Fatal(err)
	}
}
