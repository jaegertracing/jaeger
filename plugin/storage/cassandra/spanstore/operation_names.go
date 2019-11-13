// Copyright (c) 2019 The Jaeger Authors.
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

package spanstore

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)

const (
	insertOperationName       = `INSERT INTO operation_names(service_name, operation_name, span_kind) VALUES (?, ?, ?)`
	queryOperationNames       = `SELECT operation_name, span_kind FROM operation_names WHERE service_name = ? `
	queryOperationNamesByKind = `SELECT operation_name, span_kind FROM operation_names WHERE service_name = ? AND span_kind = ? `
)

// OperationNamesStorage stores known operation names by service.
type OperationNamesStorage struct {
	// CQL statements are public so that Cassandra2 storage can override them
	InsertStmt      string
	QueryStmt       string
	QueryByKindStmt string
	session         cassandra.Session
	writeCacheTTL   time.Duration
	metrics         *casMetrics.Table
	operationNames  cache.Cache
	logger          *zap.Logger
}

// NewOperationNamesStorage returns a new OperationNamesStorage
func NewOperationNamesStorage(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *OperationNamesStorage {
	return &OperationNamesStorage{
		session:         session,
		InsertStmt:      insertOperationName,
		QueryStmt:       queryOperationNames,
		QueryByKindStmt: queryOperationNamesByKind,
		metrics:         casMetrics.NewTable(metricsFactory, "operation_names"),
		writeCacheTTL:   writeCacheTTL,
		logger:          logger,
		operationNames: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL:             writeCacheTTL,
				InitialCapacity: 10000,
			}),
	}
}

// Write saves Operation and Service name and spanKind tuples
func (s *OperationNamesStorage) Write(serviceName string, operationName string, spanKind string) error {
	var err error
	query := s.session.Query(s.InsertStmt)
	if inCache := checkWriteCache(serviceName+"|"+operationName+"|"+spanKind, s.operationNames, s.writeCacheTTL); !inCache {
		q := query.Bind(serviceName, operationName, spanKind)
		err2 := s.metrics.Exec(q, s.logger)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *OperationNamesStorage) GetOperations(service string, spanKind string) ([]*storage_v1.OperationMeta, error) {
	var query cassandra.Query
	if spanKind == "" {
		// Get operations for all spanKind
		query = s.session.Query(s.QueryStmt, service)
	} else {
		// Get operations for given spanKind
		query = s.session.Query(s.QueryByKindStmt, service, spanKind)
	}
	iter := query.Iter()

	opRecord := map[string]string{}
	var operations []*storage_v1.OperationMeta
	for iter.Scan(&opRecord) {
		operations = append(operations, &storage_v1.OperationMeta{
			Operation: opRecord["operation_name"],
			SpanKind:  opRecord["span_kind"],
		})
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading operation_names from storage")
		return nil, err
	}
	return operations, nil
}
