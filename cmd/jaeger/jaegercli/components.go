// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegercli

import (
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

// Components returns the component factories that the standard Jaeger binary is
// built with. Custom distributions can add to or remove from the returned maps
// before handing the result to NewCommand, instead of transcribing Jaeger's
// component list:
//
//	factories, err := jaegercli.Components()
//	if err != nil {
//		log.Fatal(err)
//	}
//	factories.Extensions[myext.NewFactory().Type()] = myext.NewFactory()
//	err = jaegercli.NewCommand(factories).Execute()
func Components() (otelcol.Factories, error) {
	return internal.Components()
}
