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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type operationNameStorageTest struct {
	session        *mocks.Session
	writeCacheTTL  time.Duration
	metricsFactory *metricstest.Factory
	logger         *zap.Logger
	logBuffer      *testutils.Buffer
	storage        *OperationNamesStorage
}

func withOperationNamesStorage(writeCacheTTL time.Duration,
	schemaVersion schemaVersion,
	fn func(s *operationNameStorageTest)) {

	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	query := &mocks.Query{}
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[latestVersion].tableName), mock.Anything).Return(query)
	if schemaVersion != latestVersion {
		query.On("Exec").Return(errors.New("new table does not exist"))
	} else {
		query.On("Exec").Return(nil)
	}

	s := &operationNameStorageTest{
		session:        session,
		writeCacheTTL:  writeCacheTTL,
		metricsFactory: metricsFactory,
		logger:         logger,
		logBuffer:      logBuffer,
		storage:        NewOperationNamesStorage(session, writeCacheTTL, metricsFactory, logger),
	}
	fn(s)
}

func TestOperationNamesStorageWrite(t *testing.T) {
	for _, test := range []struct {
		name          string
		ttl           time.Duration
		schemaVersion schemaVersion
	}{
		{name: "test old schema with 0 ttl", ttl: 0, schemaVersion: previousVersion},
		{name: "test old schema with 1min ttl", ttl: time.Minute, schemaVersion: previousVersion},
		{name: "test new schema with 0 ttl", ttl: 0, schemaVersion: latestVersion},
		{name: "test new schema with 1min ttl", ttl: time.Minute, schemaVersion: latestVersion},
	} {
		t.Run(test.name, func(t *testing.T) {
			withOperationNamesStorage(test.ttl, test.schemaVersion, func(s *operationNameStorageTest) {
				var execError = errors.New("exec error")
				query := &mocks.Query{}
				query1 := &mocks.Query{}
				query2 := &mocks.Query{}

				if test.schemaVersion == previousVersion {
					query.On("Bind", []interface{}{"service-a", "Operation-b"}).Return(query1)
					query.On("Bind", []interface{}{"service-c", "operation-d"}).Return(query2)
				} else {
					query.On("Bind", []interface{}{"service-a", "", "Operation-b"}).Return(query1)
					query.On("Bind", []interface{}{"service-c", "", "operation-d"}).Return(query2)
				}

				query1.On("Exec").Return(nil)
				query2.On("Exec").Return(execError)
				query2.On("String").Return("select from " + schemas[test.schemaVersion].tableName)

				s.session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)

				err := s.storage.Write(dbmodel.Operation{
					ServiceName:   "service-a",
					OperationName: "Operation-b",
				})
				assert.NoError(t, err)

				err = s.storage.Write(dbmodel.Operation{
					ServiceName:   "service-c",
					OperationName: "operation-d",
				})
				assert.EqualError(t, err,
					"failed to Exec query 'select from "+
						schemas[test.schemaVersion].tableName+
						"': exec error")
				assert.Equal(t, map[string]string{
					"level": "error",
					"msg":   "Failed to exec query",
					"query": "select from " + schemas[test.schemaVersion].tableName,
					"error": "exec error",
				}, s.logBuffer.JSONLine(0))

				counts, _ := s.metricsFactory.Snapshot()
				assert.Equal(t, map[string]int64{
					"attempts|table=" + schemas[test.schemaVersion].tableName: 2,
					"inserts|table=" + schemas[test.schemaVersion].tableName:  1,
					"errors|table=" + schemas[test.schemaVersion].tableName:   1,
				}, counts, "after first two writes")

				// write again
				err = s.storage.Write(dbmodel.Operation{
					ServiceName:   "service-a",
					OperationName: "Operation-b",
				})
				assert.NoError(t, err)

				counts2, _ := s.metricsFactory.Snapshot()
				expCounts := counts
				if test.ttl == 0 {
					// without write cache, the second write must succeed
					expCounts["attempts|table="+schemas[test.schemaVersion].tableName]++
					expCounts["inserts|table="+schemas[test.schemaVersion].tableName]++
				}
				assert.Equal(t, expCounts, counts2)
			})
		})
	}
}

func TestOperationNamesStorageGetServices(t *testing.T) {
	var scanError = errors.New("scan error")
	for _, test := range []struct {
		name          string
		schemaVersion schemaVersion
		expErr        error
	}{
		{name: "test old schema without error", schemaVersion: previousVersion, expErr: nil},
		{name: "test old schema with scan error", schemaVersion: previousVersion, expErr: scanError},
		{name: "test new schema without error", schemaVersion: latestVersion, expErr: nil},
		{name: "test new schema with scan error", schemaVersion: latestVersion, expErr: scanError},
	} {
		t.Run(test.name, func(t *testing.T) {
			withOperationNamesStorage(0, test.schemaVersion, func(s *operationNameStorageTest) {
				var matched bool
				matchOnce := mock.MatchedBy(func(v []interface{}) bool {
					if matched {
						return false
					}
					matched = true
					return true
				})
				matchEverything := mock.MatchedBy(func(v []interface{}) bool { return true })

				iter := &mocks.Iterator{}
				iter.On("Scan", matchOnce).Return(true)
				iter.On("Scan", matchEverything).Return(false) // false to stop the loop
				iter.On("Close").Return(test.expErr)

				query := &mocks.Query{}
				query.On("Iter").Return(iter)

				s.session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
				services, err := s.storage.GetOperations(spanstore.OperationQueryParameters{
					ServiceName: "service-a",
				})
				if test.expErr == nil {
					assert.NoError(t, err)
					// expect one empty operation result
					// because mock iter.Scan(&placeholder) does not write to `placeholder`
					assert.Equal(t, []spanstore.Operation{{}}, services)
				} else {
					assert.EqualError(
						t,
						err,
						fmt.Sprintf("error reading %s from storage: %s",
							schemas[test.schemaVersion].tableName,
							test.expErr.Error()),
					)
				}
			})
		})

	}

}
