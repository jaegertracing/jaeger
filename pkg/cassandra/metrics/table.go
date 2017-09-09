// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/cassandra"
	storageMetrics "github.com/uber/jaeger/storage/spanstore/metrics"
)

// Table is a collection of metrics about Cassandra write operations.
type Table struct {
	storageMetrics.WriteMetrics
}

// NewTable takes a metrics scope and creates a table metrics struct
func NewTable(factory metrics.Factory, tableName string) *Table {
	t := storageMetrics.WriteMetrics{}
	metrics.Init(&t, factory.Namespace(tableName, nil), nil)
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
		return errors.Wrapf(err, "failed to Exec query '%s'", queryString)
	}
	return nil
}
