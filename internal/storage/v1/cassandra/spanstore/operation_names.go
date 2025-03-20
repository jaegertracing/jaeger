// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
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
	getOperations    func(
		s *OperationNamesStorage,
		query spanstore.OperationQueryParameters,
	) ([]spanstore.Operation, error)
}

func (t *tableMeta) materialize() {
	t.insertStmt = fmt.Sprintf(t.insertStmt, t.tableName)
	t.queryByKindStmt = fmt.Sprintf(t.queryByKindStmt, t.tableName)
	t.queryStmt = fmt.Sprintf(t.queryStmt, t.tableName)
}

var schemas = map[schemaVersion]tableMeta{
	previousVersion: {
		tableName:       "operation_names",
		insertStmt:      "INSERT INTO %s(service_name, operation_name) VALUES (?, ?)",
		queryByKindStmt: "SELECT operation_name FROM %s WHERE service_name = ?",
		queryStmt:       "SELECT operation_name FROM %s WHERE service_name = ?",
		getOperations:   getOperationsV1,
		createWriteQuery: func(query cassandra.Query, service, _ /* kind*/, opName string) cassandra.Query {
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
	table          tableMeta
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
	metricsFactory api.Factory,
	logger *zap.Logger,
) (*OperationNamesStorage, error) {
	schemaVersion := latestVersion
	if !tableExist(session, schemas[schemaVersion].tableName) {
		if !tableExist(session, schemas[previousVersion].tableName) {
			return nil, fmt.Errorf("neither table %s nor %s exist",
				schemas[schemaVersion].tableName, schemas[previousVersion].tableName)
		}
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
	}, nil
}

// Write saves Operation and Service name tuples
func (s *OperationNamesStorage) Write(operation dbmodel.Operation) error {
	key := fmt.Sprintf("%s|%s|%s",
		operation.ServiceName,
		operation.SpanKind,
		operation.OperationName,
	)
	if inCache := checkWriteCache(key, s.operationNames, s.writeCacheTTL); !inCache {
		q := s.table.createWriteQuery(
			s.session.Query(s.table.insertStmt),
			operation.ServiceName,
			operation.SpanKind,
			operation.OperationName,
		)
		err := s.metrics.Exec(q, s.logger)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *OperationNamesStorage) GetOperations(
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return s.table.getOperations(s, query)
}

func tableExist(session cassandra.Session, tableName string) bool {
	query := session.Query(fmt.Sprintf(tableCheckStmt, tableName))
	err := query.Exec()
	return err == nil
}

func getOperationsV1(
	s *OperationNamesStorage,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	iter := s.session.Query(s.table.queryStmt, query.ServiceName).Iter()

	var operation string
	var operations []spanstore.Operation
	for iter.Scan(&operation) {
		operations = append(operations, spanstore.Operation{
			Name: operation,
		})
	}
	if err := iter.Close(); err != nil {
		err = fmt.Errorf("error reading operation_names from storage: %w", err)
		return nil, err
	}

	return operations, nil
}

func getOperationsV2(
	s *OperationNamesStorage,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
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
	var operations []spanstore.Operation
	for iter.Scan(&spanKind, &operationName) {
		operations = append(operations, spanstore.Operation{
			Name:     operationName,
			SpanKind: spanKind,
		})
	}
	if err := iter.Close(); err != nil {
		err = fmt.Errorf("error reading %s from storage: %w", s.table.tableName, err)
		return nil, err
	}
	return operations, nil
}
