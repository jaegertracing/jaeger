// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/jaeger/esrollover"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	v := viper.New()
	logger, _ := zap.NewProduction()
	command := internal.Command()
	command.AddCommand(version.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(esrollover.Command(v, logger))
	config.AddFlags(
		v,
		command,
	)

	if err := command.Execute(); err != nil {
		log.Fatal(err)
	}
}
