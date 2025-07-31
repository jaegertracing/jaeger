// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import "context"

// Conn is an abstract interface representing a connection to ClickHouse.
// It is based on the driver.Conn interface in the github.com/ClickHouse/clickhouse-go package,
// primarily used for preparing Batch operations for executing ClickHouse queries.
// see https://github.com/ClickHouse/clickhouse-go/blob/main/lib/driver/driver.go#L52
type Conn interface {
	PrepareBatch(ctx context.Context, query string) (Batch, error)
}

// Batch is an abstract interface representing a Batch for bulk operations.
// It is based on the conn_batch.Batch interface in the https://github.com/ClickHouse/clickhouse-go/blob/main/conn_batch.go
// providing methods to append data to the Batch (Append) and send the Batch to ClickHouse (Send).
type Batch interface {
	Append(v ...any) error
	Send() error
}
