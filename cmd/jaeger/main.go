// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/jaegercli"
)

func main() {
	factories, err := internal.Components()
	if err != nil {
		log.Fatal(err)
	}
	if err := jaegercli.NewCommand(factories).Execute(); err != nil {
		log.Fatal(err)
	}
}
