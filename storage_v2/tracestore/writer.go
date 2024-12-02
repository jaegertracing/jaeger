// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Writer writes spans to storage.
type Writer interface {
	// WriteTrace writes a batch of spans to storage. Idempotent.
	// Implementations are not required to support atomic transactions,
	// so if any of the spans fail to be written an error is returned.
	// Compatible with OTLP Exporter API.
	WriteTraces(ctx context.Context, td ptrace.Traces) error
}
