// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Client is an abstraction collection of ch-go and clickhouse-go.
type Client interface {
	Do(ctx context.Context, query ChQuery) error
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
	Close() error
}

// ChQuery Parameters for executing WRITE operations
// Body: SQL Insert Statement
// Input: Batch Insert Data.
type ChQuery struct {
	Body  string
	Input ptrace.Traces
}
