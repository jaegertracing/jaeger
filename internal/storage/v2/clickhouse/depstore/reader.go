// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

var _ depstore.Reader = (*Reader)(nil)

type Reader struct {
	conn driver.Conn
}

func NewDependencyReader(conn driver.Conn) *Reader {
	return &Reader{conn: conn}
}

// dependencyLink is the JSON representation of a single dependency link.
type dependencyLink struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	CallCount uint64 `json:"callCount"`
}

func (r *Reader) GetDependencies(ctx context.Context, query depstore.QueryParameters) ([]model.DependencyLink, error) {
	rows, err := r.conn.Query(ctx, sql.SelectDependencies, query.StartTime, query.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query dependencies: %w", err)
	}
	defer rows.Close()

	// Merge dependencies from all snapshots in the time range.
	// Use a map keyed by (parent, child) to aggregate call counts.
	type key struct{ parent, child string }
	merged := make(map[key]uint64)

	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("failed to scan dependency row: %w", err)
		}
		var links []dependencyLink
		if err := json.Unmarshal([]byte(blob), &links); err != nil {
			return nil, fmt.Errorf("failed to unmarshal dependencies JSON: %w", err)
		}
		for _, link := range links {
			merged[key{link.Parent, link.Child}] += link.CallCount
		}
	}

	if len(merged) == 0 {
		return nil, nil
	}

	dependencies := make([]model.DependencyLink, 0, len(merged))
	for k, callCount := range merged {
		dependencies = append(dependencies, model.DependencyLink{
			Parent:    k.parent,
			Child:     k.child,
			CallCount: callCount,
		})
	}
	return dependencies, nil
}
