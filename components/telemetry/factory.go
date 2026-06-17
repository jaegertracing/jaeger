// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	coltelemetry "go.opentelemetry.io/collector/service/telemetry"

	bridge "github.com/jaegertracing/jaeger/cmd/jaeger/components/telemetry"
)

func NewFactory() coltelemetry.Factory {
	return bridge.NewFactory()
}
