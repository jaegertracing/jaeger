// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"go.opentelemetry.io/collector/exporter"

	impl "github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
)

func NewFactory() exporter.Factory {
	return impl.NewFactory()
}
