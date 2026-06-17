// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"go.opentelemetry.io/collector/extension"

	bridge "github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/remotestorage"
)

func NewFactory() extension.Factory {
	return bridge.NewFactory()
}
