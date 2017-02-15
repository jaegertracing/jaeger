// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package spanstore

import (
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/pkg/cache"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
)

const (
	insertOperationName = `INSERT INTO operation_names(service_name, operation_name) VALUES (?, ?)`
	queryOperationNames = `SELECT operation_name FROM operation_names WHERE service_name = ?`
)

// OperationNamesStorage stores known operation names by service.
type OperationNamesStorage struct {
	// CQL statements are public so that Cassandra2 storage can override them
	InsertStmt     string
	QueryStmt      string
	session        cassandra.Session
	writeCacheTTL  time.Duration
	metrics        *casMetrics.Table
	operationNames cache.Cache
	logger         zap.Logger
}

// NewOperationNamesStorage returns a new OperationNamesStorage
func NewOperationNamesStorage(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger zap.Logger,
) *OperationNamesStorage {
	return &OperationNamesStorage{
		session:       session,
		InsertStmt:    insertOperationName,
		QueryStmt:     queryOperationNames,
		metrics:       casMetrics.NewTable(metricsFactory, "OperationNames"),
		writeCacheTTL: writeCacheTTL,
		logger:        logger,
		operationNames: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL:             writeCacheTTL,
				InitialCapacity: 0000,
			}),
	}
}

// Write saves Operation and Service name tuples
func (s *OperationNamesStorage) Write(serviceName string, operationName string) error {
	operationName = strings.ToLower(operationName)
	var err error
	query := s.session.Query(s.InsertStmt)
	if inCache := checkWriteCache(serviceName+"|"+operationName, s.operationNames, s.writeCacheTTL); !inCache {
		q := query.Bind(serviceName, operationName)
		err2 := s.metrics.Exec(q, s.logger)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *OperationNamesStorage) GetOperations(service string) ([]string, error) {
	iter := s.session.Query(s.QueryStmt, service).Iter()

	var operation string
	var operations []string
	for iter.Scan(&operation) {
		operations = append(operations, operation)
	}
	if err := iter.Close(); err != nil {
		err = errors.Wrap(err, "Error reading operation_names from storage")
		return nil, err
	}
	return operations, nil
}
