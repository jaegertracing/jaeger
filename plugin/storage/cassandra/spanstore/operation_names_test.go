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
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/testutils"
)

type operationNameStorageTest struct {
	session        *mocks.Session
	writeCacheTTL  time.Duration
	metricsFactory *metrics.LocalFactory
	logger         *zap.Logger
	logBuffer      *testutils.Buffer
	storage        *OperationNamesStorage
}

func withOperationNamesStorage(writeCacheTTL time.Duration, fn func(s *operationNameStorageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metrics.NewLocalFactory(0)
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
	for _, ttl := range []time.Duration{0, time.Minute} {
		writeCacheTTL := ttl // capture loop var
		t.Run(fmt.Sprintf("writeCacheTTL=%v", writeCacheTTL), func(t *testing.T) {
			withOperationNamesStorage(writeCacheTTL, func(s *operationNameStorageTest) {
				var execError = errors.New("exec error")
				query := &mocks.Query{}
				query1 := &mocks.Query{}
				query2 := &mocks.Query{}
				query.On("Bind", []interface{}{"service-a", "Operation-b"}).Return(query1)
				query.On("Bind", []interface{}{"service-c", "operation-d"}).Return(query2)
				query1.On("Exec").Return(nil)
				query2.On("Exec").Return(execError)
				query2.On("String").Return("select from operation_names")

				var emptyArgs []interface{}
				s.session.On("Query", mock.AnythingOfType("string"), emptyArgs).Return(query)

				err := s.storage.Write("service-a", "Operation-b")
				assert.NoError(t, err)

				err = s.storage.Write("service-c", "operation-d")
				assert.EqualError(t, err, "failed to Exec query 'select from operation_names': exec error")
				assert.Equal(t, map[string]string{
					"level": "error",
					"msg":   "Failed to exec query",
					"query": "select from operation_names",
					"error": "exec error",
				}, s.logBuffer.JSONLine(0))

				counts, _ := s.metricsFactory.Snapshot()
				assert.Equal(t, map[string]int64{
					"OperationNames.attempts": 2, "OperationNames.inserts": 1, "OperationNames.errors": 1,
				}, counts, "after first two writes")

				// write again
				err = s.storage.Write("service-a", "Operation-b")
				assert.NoError(t, err)

				counts2, _ := s.metricsFactory.Snapshot()
				expCounts := counts
				if writeCacheTTL == 0 {
					// without write cache, the second write must succeed
					expCounts["OperationNames.attempts"]++
					expCounts["OperationNames.inserts"]++
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
	for _, expErr := range []error{nil, scanError} {
		withOperationNamesStorage(writeCacheTTL, func(s *operationNameStorageTest) {
			iter := &mocks.Iterator{}
			iter.On("Scan", matchOnce).Return(true)
			iter.On("Scan", matchEverything).Return(false) // false to stop the loop
			iter.On("Close").Return(expErr)

			query := &mocks.Query{}
			query.On("Iter").Return(iter)

			s.session.On("Query", mock.AnythingOfType("string"), []interface{}{"service-a"}).Return(query)

			services, err := s.storage.GetOperations("service-a")
			if expErr == nil {
				assert.NoError(t, err)
				// expect empty string because mock iter.Scan(&placeholder) does not write to `placeholder`
				assert.Equal(t, []string{""}, services)
			} else {
				assert.EqualError(t, err, "Error reading operation_names from storage: "+expErr.Error())
			}
		})

	}

}
