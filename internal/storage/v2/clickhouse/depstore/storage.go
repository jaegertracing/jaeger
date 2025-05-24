package depstore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

var _ depstore.Reader = (*Storage)(nil)

type Storage struct {
	conn driver.Conn
}

func (s *Storage) GetDependencies(
	ctx context.Context,
	query depstore.QueryParameters,
) ([]model.DependencyLink, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT parent, child, call_count, source
		FROM dependencies
		WHERE timestamp >= ? AND timestamp <= ?
	`,
		query.StartTime,
		query.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query dependencies: %w", err)
	}
	defer rows.Close()

	var dependencies []model.DependencyLink
	for rows.Next() {
		var dependency Dependency
		if err := rows.ScanStruct(dependency); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		dependencies = append(dependencies, dependency.ToModel())
	}

	return dependencies, nil
}
