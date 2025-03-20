// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
)

// Table is a collection of metrics about Cassandra write operations.
type Table struct {
	spanstoremetrics.WriteMetrics
}

// NewTable takes a metrics scope and creates a table metrics struct
func NewTable(factory api.Factory, tableName string) *Table {
	t := spanstoremetrics.WriteMetrics{}
	api.Init(&t, factory.Namespace(api.NSOptions{Name: "", Tags: map[string]string{"table": tableName}}), nil)
	return &Table{t}
}

// Exec executes an update query and reports metrics/logs about it.
func (t *Table) Exec(query cassandra.UpdateQuery, logger *zap.Logger) error {
	start := time.Now()
	err := query.Exec()
	t.Emit(err, time.Since(start))
	if err != nil {
		queryString := query.String()
		if logger != nil {
			logger.Error("Failed to exec query", zap.String("query", queryString), zap.Error(err))
		}
		return fmt.Errorf("failed to Exec query '%s': %w", queryString, err)
	}
	return nil
}
