// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// jaeger-e2e is the binary the e2e integration harness spawns: the standard
// Jaeger component set plus the storage_cleaner extension, which serves an
// unauthenticated endpoint that deletes every trace in the configured backend.
// The harness needs it to reset storage between suites, and no production
// binary should carry it, so it is registered here instead of in
// internal.Components().
package main

import (
	"log"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
	"github.com/jaegertracing/jaeger/cmd/jaeger/jaegercli"
)

func main() {
	factories, err := internal.Components()
	if err != nil {
		log.Fatal(err)
	}
	cleaner := storagecleaner.NewFactory()
	factories.Extensions[cleaner.Type()] = cleaner
	if err := jaegercli.NewCommand(factories).Execute(); err != nil {
		log.Fatal(err)
	}
}
