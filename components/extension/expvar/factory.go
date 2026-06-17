// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expvar

import (
	"go.opentelemetry.io/collector/extension"

	bridge "github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/expvar"
)

func NewFactory() extension.Factory {
	return bridge.NewFactory()
}
