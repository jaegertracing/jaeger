// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
)

// Conn is an s abstraction of clickhouse.Conn
type Conn interface {
	PrepareBatch(ctx context.Context, query string) (Batch, error)
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	Close() error
}

type Batch interface {
	Append(v ...any) error
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}
