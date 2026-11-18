// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

var _ depstore.Writer = (*Writer)(nil)

type Writer struct {
	conn driver.Conn
}

func NewDependencyWriter(conn driver.Conn) *Writer {
	return &Writer{conn: conn}
}

func (w *Writer) WriteDependencies(ctx context.Context, ts time.Time, dependencies []model.DependencyLink) error {
	jsonBytes, err := json.Marshal(dependencyLinksFromModel(dependencies))
	if err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}
	// distributed_foreground_insert makes an insert into a Distributed table wait for its
	// rows to reach their shard before acknowledging; the ClickHouse default queues them
	// for background delivery instead. It has no effect on a non-Distributed table. It is
	// a query setting, not a connection setting: a connection that carries it is refused
	// by a read-only ClickHouse user, even on a query that changes nothing.
	ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"distributed_foreground_insert": 1}))
	batch, err := w.conn.PrepareBatch(ctx, sql.InsertDependencies)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}
	defer batch.Close()
	if err := batch.Append(ts, string(jsonBytes)); err != nil {
		return fmt.Errorf("failed to append to batch: %w", err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	return nil
}
