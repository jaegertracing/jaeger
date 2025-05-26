// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

type Reader struct {
	conn driver.Conn
}

func NewReader(conn driver.Conn) *Reader {
	return &Reader{conn: conn}
}

func (r *Reader) GetServices(ctx context.Context) ([]string, error) {
	rows, err := r.conn.Query(ctx, `SELECT name FROM services`)
	if err != nil {
		return nil, fmt.Errorf("failed to query services: %w", err)
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var service dbmodel.Service
		if err := rows.ScanStruct(&service); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		services = append(services, service.Name)
	}
	return services, nil
}
