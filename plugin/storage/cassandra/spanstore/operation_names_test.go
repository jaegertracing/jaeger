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
)

type test struct {
	ttl           time.Duration
	schemaVersion int
	expErr        error
}

type operationNameStorageTest struct {
	session        *mocks.Session
	writeCacheTTL  time.Duration
	metricsFactory *metricstest.Factory
	logger         *zap.Logger
	logBuffer      *testutils.Buffer
	storage        *OperationNamesStorage
}

func withOperationNamesStorage(writeCacheTTL time.Duration, schemaVersion int, fn func(s *operationNameStorageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	query := &mocks.Query{}
	session.On("Query", fmt.Sprintf(TableQueryStmt, schemas[LatestVersion].TableName), mock.Anything).Return(query)
	if schemaVersion != LatestVersion {
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
	for _, test := range []test{
		{0, 0, nil},
		{time.Minute, 0, nil},
		{0, 1, nil},
		{time.Minute, 1, nil},
	} {
		writeCacheTTL := test.ttl // capture loop var
		t.Run(fmt.Sprintf("test %#v", test), func(t *testing.T) {
			withOperationNamesStorage(writeCacheTTL, test.schemaVersion, func(s *operationNameStorageTest) {
				var execError = errors.New("exec error")
				query := &mocks.Query{}
				query1 := &mocks.Query{}
				query2 := &mocks.Query{}

				if test.schemaVersion == 0 {
					query.On("Bind", []interface{}{"service-a", "Operation-b"}).Return(query1)
					query.On("Bind", []interface{}{"service-c", "operation-d"}).Return(query2)
				} else {
					query.On("Bind", []interface{}{"service-a", "", "Operation-b"}).Return(query1)
					query.On("Bind", []interface{}{"service-c", "", "operation-d"}).Return(query2)
				}

				query1.On("Exec").Return(nil)
				query2.On("Exec").Return(execError)
				query2.On("String").Return("select from " + schemas[test.schemaVersion].TableName)

				s.session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)

				err := s.storage.Write("service-a", "Operation-b")
				assert.NoError(t, err)

				err = s.storage.Write("service-c", "operation-d")
				assert.EqualError(t, err, "failed to Exec query 'select from "+schemas[test.schemaVersion].TableName+"': exec error")
				assert.Equal(t, map[string]string{
					"level": "error",
					"msg":   "Failed to exec query",
					"query": "select from " + schemas[test.schemaVersion].TableName,
					"error": "exec error",
				}, s.logBuffer.JSONLine(0))

				counts, _ := s.metricsFactory.Snapshot()
				assert.Equal(t, map[string]int64{
					"attempts|table=" + schemas[test.schemaVersion].TableName: 2, "inserts|table=" + schemas[test.schemaVersion].TableName: 1, "errors|table=" + schemas[test.schemaVersion].TableName: 1,
				}, counts, "after first two writes")

				// write again
				err = s.storage.Write("service-a", "Operation-b")
				assert.NoError(t, err)

				counts2, _ := s.metricsFactory.Snapshot()
				expCounts := counts
				if writeCacheTTL == 0 {
					// without write cache, the second write must succeed
					expCounts["attempts|table="+schemas[test.schemaVersion].TableName]++
					expCounts["inserts|table="+schemas[test.schemaVersion].TableName]++
				}
				assert.Equal(t, expCounts, counts2)
			})
		})
	}
}

func TestOperationNamesStorageGetServices(t *testing.T) {
	var scanError = errors.New("scan error")
	var writeCacheTTL time.Duration
	var matched bool
	matchOnce := mock.MatchedBy(func(v []interface{}) bool {
		if matched {
			return false
		}
		matched = true
		return true
	})
	matchEverything := mock.MatchedBy(func(v []interface{}) bool { return true })
	for _, test := range []test{
		{0, 0, nil},
		{0, 0, scanError},
		{0, 1, nil},
		{0, 1, scanError},
	} {
		t.Run(fmt.Sprintf("test %#v", test), func(t *testing.T) {
			withOperationNamesStorage(writeCacheTTL, test.schemaVersion, func(s *operationNameStorageTest) {
				iter := &mocks.Iterator{}
				iter.On("Scan", matchOnce).Return(true)
				iter.On("Scan", matchEverything).Return(false) // false to stop the loop
				iter.On("Close").Return(test.expErr)

				query := &mocks.Query{}
				query.On("Iter").Return(iter)

				s.session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
				services, err := s.storage.GetOperations("service-a")
				if test.expErr == nil {
					assert.NoError(t, err)
					if test.schemaVersion == 0 {
						// expect one empty operation result because mock iter.Scan(&placeholder) does not write to `placeholder`
						assert.Equal(t, []string{""}, services)
					} else {
						assert.Equal(t, []string{}, services)
					}
				} else {
					assert.EqualError(t, err, fmt.Sprintf("Error reading %s from storage: %s", schemas[test.schemaVersion].TableName, test.expErr.Error()))
				}
			})
		})
	}

}
