// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"go.opentelemetry.io/collector/exporter"

	bridge "github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/storageexporter"
)

func NewFactory() exporter.Factory {
	return bridge.NewFactory()
}
