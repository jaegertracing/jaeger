// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"go.opentelemetry.io/collector/processor"

	bridge "github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/adaptivesampling"
)

func NewFactory() processor.Factory {
	return bridge.NewFactory()
}
