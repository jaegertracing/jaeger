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
