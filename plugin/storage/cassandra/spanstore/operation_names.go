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
	// LatestVersion latest version of operation_names table schema, increase the version if your table schema changes require code change
	LatestVersion = 1
	// TableQueryStmt the query statement used to check if a table exists or not
	TableQueryStmt = "SELECT * from %s limit 1"
)

type schemaMeta struct {
	TableName       string
	InsertStmt      string
	QueryByKindStmt string
	QueryStmt       string
}

var schemas = []schemaMeta{
	{
		TableName:       "operation_names",
		InsertStmt:      "INSERT INTO %s(service_name, operation_name) VALUES (?, ?)",
		QueryByKindStmt: "SELECT operation_name FROM %s WHERE service_name = ?",
		QueryStmt:       "SELECT operation_name FROM %s WHERE service_name = ?",
	},
	{
		TableName:       "operation_names_v2",
		InsertStmt:      "INSERT INTO %s(service_name, span_kind, operation_name) VALUES (?, ?, ?)",
		QueryByKindStmt: "SELECT span_kind, operation_name FROM %s WHERE service_name = ? AND span_kind = ?",
		QueryStmt:       "SELECT span_kind, operation_name FROM %s WHERE service_name = ?",
	},
}

// OperationNamesStorage stores known operation names by service.
type OperationNamesStorage struct {
	// CQL statements are public so that Cassandra2 storage can override them
	SchemaVersion   int
	TableName       string
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

	schemaVersion := LatestVersion

	if !tableExist(session, schemas[schemaVersion].TableName) {
		schemaVersion = schemaVersion - 1
	}

	return &OperationNamesStorage{
		session:         session,
		TableName:       schemas[schemaVersion].TableName,
		SchemaVersion:   schemaVersion,
		InsertStmt:      fmt.Sprintf(schemas[schemaVersion].InsertStmt, schemas[schemaVersion].TableName),
		QueryByKindStmt: fmt.Sprintf(schemas[schemaVersion].QueryByKindStmt, schemas[schemaVersion].TableName),
		QueryStmt:       fmt.Sprintf(schemas[schemaVersion].QueryStmt, schemas[schemaVersion].TableName),
		metrics:         casMetrics.NewTable(metricsFactory, schemas[schemaVersion].TableName),
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

func tableExist(session cassandra.Session, tableName string) bool {
	query := session.Query(fmt.Sprintf(TableQueryStmt, tableName))
	err := query.Exec()
	return err == nil
}

// Write saves Operation and Service name tuples
func (s *OperationNamesStorage) Write(serviceName string, operationName string) error {
	var err error
	//TODO: take spanKind from args
	spanKind := ""
	query := s.session.Query(s.InsertStmt)
	if inCache := checkWriteCache(serviceName+"|"+spanKind+"|"+operationName, s.operationNames, s.writeCacheTTL); !inCache {
		var q cassandra.Query
		switch s.SchemaVersion {
		case 1:
			q = query.Bind(serviceName, spanKind, operationName)
		case 0:
			q = query.Bind(serviceName, operationName)
		}

		err2 := s.metrics.Exec(q, s.logger)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *OperationNamesStorage) GetOperations(service string) ([]string, error) {
	var operations []*spanstore.Operation
	var err error

	switch s.SchemaVersion {
	case 1:
		operations, err = getOperationsV1(s, &spanstore.OperationQueryParameters{
			ServiceName: service,
		})
	case 0:
		operations, err = getOperationsV0(s, service)
	}

	if err != nil {
		return nil, err
	}
	operationNames := make([]string, len(operations))
	for idx, operation := range operations {
		operationNames[idx] = operation.Name
	}
	return operationNames, err
}

func getOperationsV0(s *OperationNamesStorage, service string) ([]*spanstore.Operation, error) {
	iter := s.session.Query(s.QueryStmt, service).Iter()

	var operation string
	var operationNames []string
	for iter.Scan(&operation) {
		operationNames = append(operationNames, operation)
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading operation_names from storage")
		return nil, err
	}

	operations := make([]*spanstore.Operation, len(operationNames))
	for idx, name := range operationNames {
		operations[idx] = &spanstore.Operation{
			Name: name,
		}
	}
	return operations, nil
}

func getOperationsV1(s *OperationNamesStorage, query *spanstore.OperationQueryParameters) ([]*spanstore.Operation, error) {
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
	var operations []*spanstore.Operation
	for iter.Scan(&spanKind, &operationName) {
		operations = append(operations, &spanstore.Operation{
			Name:     operationName,
			SpanKind: spanKind,
		})
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("Error reading %s from storage", s.TableName))
		return nil, err
	}
	return operations, nil
}
