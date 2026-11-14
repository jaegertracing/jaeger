// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

// checkAttributeLayout verifies that the spans table stores its attribute groups in the
// configured layout. The table may have been created by the operator or under an earlier
// configuration, and the statements of one layout fail on a table of the other only when the
// first span is written or read. A table that does not exist passes the check.
func checkAttributeLayout(ctx context.Context, conn driver.Conn, configured chtracestore.AttributeLayout) error {
	rows, err := conn.Query(ctx, sql.SelectAttributeLayout)
	if err != nil {
		return fmt.Errorf("failed to read the attribute layout of the spans table: %w", err)
	}
	defer rows.Close()
	// The rows are read to the end, because the driver reports an error of the result stream
	// only after the last row.
	var stored chtracestore.AttributeLayout
	for rows.Next() {
		var isMap uint8
		if err := rows.Scan(&isMap); err != nil {
			return fmt.Errorf("failed to read the attribute layout of the spans table: %w", err)
		}
		stored = chtracestore.AttributeLayoutNested
		if isMap != 0 {
			stored = chtracestore.AttributeLayoutMap
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read the attribute layout of the spans table: %w", err)
	}
	if stored != "" && stored != configured {
		return fmt.Errorf("the spans table stores attributes in the %s layout, and attribute_layout is %s; "+
			"an existing table is not converted, so set attribute_layout to %s or create the tables in a new database",
			stored, configured, stored)
	}
	return nil
}
