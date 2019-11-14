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
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	operationTableName        = `operations`
	insertOperationName       = `INSERT INTO ` + operationTableName + ` (service_name, span_kind, operation_name) VALUES (?, ?, ?)`
	queryOperationNames       = `SELECT span_kind, operation_name FROM ` + operationTableName + ` WHERE service_name = ? `
	queryOperationNamesByKind = `SELECT span_kind, operation_name FROM ` + operationTableName + ` WHERE service_name = ? AND span_kind = ? `
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
		metrics:         casMetrics.NewTable(metricsFactory, operationTableName),
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
	if inCache := checkWriteCache(serviceName+"|"+spanKind+"|"+operationName, s.operationNames, s.writeCacheTTL); !inCache {
		q := query.Bind(serviceName, spanKind, operationName)
		err2 := s.metrics.Exec(q, s.logger)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *OperationNamesStorage) GetOperations(query *spanstore.OperationQueryParameters) ([]*storage_v1.Operation, error) {
	var casQuery cassandra.Query
	if query.SpanKind == "" {
		// Get operations for all spanKind
		casQuery = s.session.Query(s.QueryStmt, query.ServiceName)
	} else {
		// Get operations for given spanKind
		casQuery = s.session.Query(s.QueryByKindStmt, query.ServiceName, query.SpanKind)
	}
	iter := casQuery.Iter()

	var operationName string
	var spanKind string
	var operations []*storage_v1.Operation
	for iter.Scan(&spanKind, &operationName) {
		operations = append(operations, &storage_v1.Operation{
			Name:     operationName,
			SpanKind: spanKind,
		})
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("Error reading %s from storage", operationTableName))
		return nil, err
	}
	return operations, nil
}
