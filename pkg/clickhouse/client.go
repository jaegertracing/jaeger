// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Client is an abstraction collection of ch-go and clickhouse-go.
type Client interface {
	Do(ctx context.Context, query QueryParam) error
	Close() error
}

// QueryParam Parameters for executing write operations
// Body: SQL Insert Statement
// Input: Batch Insert Data.
type QueryParam struct {
	Body  string
	Input ptrace.Traces
}
