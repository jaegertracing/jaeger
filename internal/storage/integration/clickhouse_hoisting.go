// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"

	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
)

// ClickHouseHoistedDatabase is the database of the runs with hoisted attributes. Those runs
// need tables of their own, because the backend never adds a column to an existing table.
const ClickHouseHoistedDatabase = "jaeger_hoisted"

// ClickHouseHoistedAttributes hoists a subset of the corpus attributes that the tag and filter
// searches look for: all four scalar types, a key present at both levels, a key stored with
// two types, and a key whose only value is zero. The last entry is in no span; its key needs
// escaping and its column name is a ClickHouse keyword, which a real server has to accept.
var ClickHouseHoistedAttributes = []ch.HoistedAttribute{
	{
		Key:    "sameplacetag1",
		Level:  "span",
		Type:   "string",
		Column: "span_tag1",
	},
	{
		Key:    "sameplacetag2",
		Level:  "span",
		Type:   "int",
		Column: "span_tag2",
	},
	{
		Key:    "sameplacetag3",
		Level:  "span",
		Type:   "double",
		Column: "span_tag3",
	},
	{
		Key:    "sameplacetag4",
		Level:  "span",
		Type:   "bool",
		Column: "span_tag4",
	},
	{
		Key:    "sameplacetag1",
		Level:  "resource",
		Type:   "string",
		Column: "resource_tag1",
	},
	{
		Key:    "zone",
		Level:  "span",
		Type:   "string",
		Column: "span_zone",
	},
	{
		Key:    "zone",
		Level:  "resource",
		Type:   "string",
		Column: "resource_zone",
	},
	{
		Key:    "retry.count",
		Level:  "span",
		Type:   "int",
		Column: "retry_count",
		Codec:  "ZSTD(3)",
	},
	{
		Key:    "queue.depth",
		Level:  "span",
		Type:   "int",
		Column: "queue_depth",
	},
	{
		Key:    "deployment.environment",
		Level:  "resource",
		Type:   "string",
		Column: "deployment_environment",
	},
	{
		Key:    `it's a \key`,
		Level:  "span",
		Type:   "string",
		Column: "index",
	},
}

// RecreateClickHouseDatabase creates an empty database beside the one the compose file creates.
// It drops a database left by an earlier run, whose tables may have other hoisted columns.
func RecreateClickHouseDatabase(t *testing.T, name string) {
	t.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"127.0.0.1:9000"},
		Auth: clickhouse.Auth{
			Username: "default",
			Password: "password",
		},
	})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" SYNC"))
	require.NoError(t, conn.Exec(context.Background(), "CREATE DATABASE "+name))
}
