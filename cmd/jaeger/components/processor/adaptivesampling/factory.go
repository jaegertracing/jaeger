// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"go.opentelemetry.io/collector/processor"

	impl "github.com/jaegertracing/jaeger/cmd/jaeger/internal/processors/adaptivesampling"
)

func NewFactory() processor.Factory {
	return impl.NewFactory()
}
