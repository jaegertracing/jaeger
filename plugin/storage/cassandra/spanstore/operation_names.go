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
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	// latestVersion of operation_names table
	// increase the version if your table schema changes require code change
	latestVersion = schemaVersion("v2")

	// previous version of operation_names table
	// if latest version does not work, will fail back to use previous version
	previousVersion = schemaVersion("v1")

	// tableCheckStmt the query statement used to check if a table exists or not
	tableCheckStmt = "SELECT * from %s limit 1"
)

type schemaVersion string

type tableMeta struct {
	tableName        string
	insertStmt       string
	queryByKindStmt  string
	queryStmt        string
	createWriteQuery func(query cassandra.Query, service, kind, opName string) cassandra.Query
	getOperations    func(s *OperationNamesStorage, query *spanstore.OperationQueryParameters) ([]*spanstore.Operation, error)
}

func (t *tableMeta) materialize() {
	t.insertStmt = fmt.Sprintf(t.insertStmt, t.tableName)
	t.queryByKindStmt = fmt.Sprintf(t.queryByKindStmt, t.tableName)
	t.queryStmt = fmt.Sprintf(t.queryStmt, t.tableName)
}

var schemas = map[schemaVersion]*tableMeta{
	previousVersion: {
		tableName:       "operation_names",
		insertStmt:      "INSERT INTO %s(service_name, operation_name) VALUES (?, ?)",
		queryByKindStmt: "SELECT operation_name FROM %s WHERE service_name = ?",
		queryStmt:       "SELECT operation_name FROM %s WHERE service_name = ?",
		getOperations:   getOperationsV1,
		createWriteQuery: func(query cassandra.Query, service, kind, opName string) cassandra.Query {
			return query.Bind(service, opName)
		},
	},
	latestVersion: {
		tableName:       "operation_names_v2",
		insertStmt:      "INSERT INTO %s(service_name, span_kind, operation_name) VALUES (?, ?, ?)",
		queryByKindStmt: "SELECT span_kind, operation_name FROM %s WHERE service_name = ? AND span_kind = ?",
		queryStmt:       "SELECT span_kind, operation_name FROM %s WHERE service_name = ?",
		getOperations:   getOperationsV2,
		createWriteQuery: func(query cassandra.Query, service, kind, opName string) cassandra.Query {
			return query.Bind(service, kind, opName)
		},
	},
}

// OperationNamesStorage stores known operation names by service.
type OperationNamesStorage struct {
	// CQL statements are public so that Cassandra2 storage can override them
	schemaVersion  schemaVersion
	table          *tableMeta
	session        cassandra.Session
	writeCacheTTL  time.Duration
	metrics        *casMetrics.Table
	operationNames cache.Cache
	logger         *zap.Logger
}

// NewOperationNamesStorage returns a new OperationNamesStorage
func NewOperationNamesStorage(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *OperationNamesStorage {

	schemaVersion := latestVersion
	if !tableExist(session, schemas[schemaVersion].tableName) {
		schemaVersion = previousVersion
	}
	table := schemas[schemaVersion]
	table.materialize()

	return &OperationNamesStorage{
		session:       session,
		schemaVersion: schemaVersion,
		table:         table,
		metrics:       casMetrics.NewTable(metricsFactory, schemas[schemaVersion].tableName),
		writeCacheTTL: writeCacheTTL,
		logger:        logger,
		operationNames: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL:             writeCacheTTL,
				InitialCapacity: 10000,
			}),
	}
}

// Write saves Operation and Service name tuples
func (s *OperationNamesStorage) Write(serviceName, operationName, spanKind string) error {
	var err error

	if inCache := checkWriteCache(serviceName+"|"+spanKind+"|"+operationName, s.operationNames, s.writeCacheTTL); !inCache {
		q := s.table.createWriteQuery(s.session.Query(s.table.insertStmt), serviceName, spanKind, operationName)
		err2 := s.metrics.Exec(q, s.logger)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *OperationNamesStorage) GetOperations(query *spanstore.OperationQueryParameters) ([]*spanstore.Operation, error) {
	return s.table.getOperations(s, &spanstore.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
}

func tableExist(session cassandra.Session, tableName string) bool {
	query := session.Query(fmt.Sprintf(tableCheckStmt, tableName))
	err := query.Exec()
	return err == nil
}

func getOperationsV1(s *OperationNamesStorage, query *spanstore.OperationQueryParameters) ([]*spanstore.Operation, error) {
	iter := s.session.Query(s.table.queryStmt, query.ServiceName).Iter()

	var operation string
	var operations []*spanstore.Operation
	for iter.Scan(&operation) {
		operations = append(operations, &spanstore.Operation{
			Name: operation,
		})
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading operation_names from storage")
		return nil, err
	}

	return operations, nil
}

func getOperationsV2(s *OperationNamesStorage, query *spanstore.OperationQueryParameters) ([]*spanstore.Operation, error) {
	var casQuery cassandra.Query
	if query.SpanKind == "" {
		// Get operations for all spanKind
		casQuery = s.session.Query(s.table.queryStmt, query.ServiceName)
	} else {
		// Get operations for given spanKind
		casQuery = s.session.Query(s.table.queryByKindStmt, query.ServiceName, query.SpanKind)
	}
	iter := casQuery.Iter()

	var operationName string
	var spanKind string
	var operations []*spanstore.Operation
	for iter.Scan(&spanKind, &operationName) {
		operations = append(operations, &spanstore.Operation{
			Name:     operationName,
			SpanKind: spanKind,
		})
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("Error reading %s from storage", s.table.tableName))
		return nil, err
	}
	return operations, nil
}
