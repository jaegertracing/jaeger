// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"go.opentelemetry.io/collector/extension"

	impl "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
)

func NewFactory() extension.Factory {
	return impl.NewFactory()
}
