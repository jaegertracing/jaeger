// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	createSpansTableSQL = `CREATE TABLE IF NOT EXISTS %s (
		Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
		TraceId String CODEC(ZSTD(1)),
		SpanId String CODEC(ZSTD(1)),
		ParentSpanId String CODEC(ZSTD(1)),
		Operation LowCardinality(String) CODEC(ZSTD(1)),
		Service LowCardinality(String) CODEC(ZSTD(1)),
		Tags Nested
		(
		   keys LowCardinality(String),
		   values String
		) CODEC (ZSTD(1)),
		Duration UInt64 CODEC(ZSTD(1)),
		INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
		INDEX idx_tags_keys Tags.keys TYPE bloom_filter(0.01) GRANULARITY 1,
		INDEX idx_tags_values Tags.values TYPE bloom_filter(0.01) GRANULARITY 1,
		INDEX idx_duration Duration TYPE minmax GRANULARITY 1
   ) ENGINE MergeTree()

   PARTITION BY toDate(Timestamp)
   ORDER BY (Service, Operation, toUnixTimestamp(Timestamp), Duration, TraceId)
   SETTINGS index_granularity=8192, ttl_only_drop_parts = 1;
	`
)

func CreateSpansTable(ctx context.Context, db *sql.DB, tableName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(createSpansTableSQL, tableName))
	return err
}
