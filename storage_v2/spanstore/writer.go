// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Writer writes spans to storage.
type Writer interface {
	// WriteTrace writes batches of spans at once and
	// compatible with OTLP Exporter API.
	WriteTraces(ctx context.Context, td ptrace.Traces) error
}
