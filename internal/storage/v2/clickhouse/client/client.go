// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type ChPool interface {
	Do(ctx context.Context, query string, td ptrace.Traces) error
	Close() error
}

type Clickhouse interface {
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	Close() error
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
}
