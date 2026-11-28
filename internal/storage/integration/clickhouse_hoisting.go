// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
)

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
	conn := openClickHouseWithAuth(t, []string{"127.0.0.1:9000"}, "", "default", "password")
	defer conn.Close()
	require.NoError(t, conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" SYNC"))
	require.NoError(t, conn.Exec(context.Background(), "CREATE DATABASE "+name))
}

func openClickHouseWithAuth(t *testing.T, addresses []string, database, username, password string) driver.Conn {
	t.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: addresses,
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
	})
	require.NoError(t, err)
	return conn
}

// TruncateClickHouseLocalTables clears all backing *_local tables in database via a direct
// ClickHouse connection. Needed in distributed mode because TRUNCATE on a Distributed table only
// discards pending async inserts rather than data in the underlying local tables. Table
// identifiers are compile-time constants; ClickHouse DDL does not admit parameterized
// identifiers, and a Replicated database rejects ON CLUSTER while propagating TRUNCATE to replicas.
func TruncateClickHouseLocalTables(t *testing.T, addresses []string, database, username, password string) {
	t.Helper()
	conn := openClickHouseWithAuth(t, addresses, database, username, password)
	defer conn.Close()
	tables := []string{
		"spans_local",
		"services_local",
		"operations_local",
		"trace_id_timestamps_local",
		"attribute_metadata_local",
		"dependencies_local",
	}
	for _, tbl := range tables {
		require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("TRUNCATE TABLE IF EXISTS %s.%s", database, tbl)))
	}
}
