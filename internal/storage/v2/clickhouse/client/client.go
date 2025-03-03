// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/conn"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/pool"
)

type Pool interface {
	Do(ctx context.Context, query string, td ...ptrace.Traces) error
	Close() error
}

type Chpool interface {
	Dial(config pool.Configuration, log *zap.Logger) (Pool, error)
}

type Conn interface {
	// TODO arg should support the dyment parameter.
	Query(ctx context.Context, query string, arg string) (Rows, error)
	Exec(ctx context.Context, query string) error
	Close() error
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	ScanStruct(dest any) error
	Err() error
}

type Clickhouse interface {
	Open(config conn.Configuration) (Conn, error)
}
