// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	coltelemetry "go.opentelemetry.io/collector/service/telemetry"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"

	jaegerinternal "github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

func NewFactory() coltelemetry.Factory {
	return jaegerinternal.WrapFactory(otelconftelemetry.NewFactory())
}
