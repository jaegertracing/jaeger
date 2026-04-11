// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

func (w *Writer) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	jsonBytes, err := json.Marshal(dependencyLinksFromModel(dependencies))
	if err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}
	ctx := context.TODO()
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
