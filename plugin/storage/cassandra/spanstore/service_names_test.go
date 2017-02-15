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
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/testutils"
)

type serviceNameStorageTest struct {
	session        *mocks.Session
	writeCacheTTL  time.Duration
	metricsFactory *metrics.LocalFactory
	logger         zap.Logger
	logBuffer      *bytes.Buffer
	storage        *ServiceNamesStorage
}

func withServiceNamesStorage(writeCacheTTL time.Duration, fn func(s *serviceNameStorageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger(false)
	metricsFactory := metrics.NewLocalFactory(time.Second)
	defer metricsFactory.Stop()
	s := &serviceNameStorageTest{
		session:        session,
		writeCacheTTL:  writeCacheTTL,
		metricsFactory: metricsFactory,
		logger:         logger,
		logBuffer:      logBuffer,
		storage:        NewServiceNamesStorage(session, writeCacheTTL, metricsFactory, logger),
	}
	fn(s)
}

func TestServiceNamesStorageWrite(t *testing.T) {
	for _, ttl := range []time.Duration{0, time.Minute} {
		writeCacheTTL := ttl // capture loop var
		t.Run(fmt.Sprintf("writeCacheTTL=%v", writeCacheTTL), func(t *testing.T) {
			withServiceNamesStorage(writeCacheTTL, func(s *serviceNameStorageTest) {
				var execError = errors.New("exec error")
				query := &mocks.Query{}
				query1 := &mocks.Query{}
				query2 := &mocks.Query{}
				query.On("Bind", []interface{}{"service-a"}).Return(query1)
				query.On("Bind", []interface{}{"service-b"}).Return(query2)
				query1.On("Exec").Return(nil)
				query2.On("Exec").Return(execError)
				query2.On("String").Return("select from service_names")

				var emptyArgs []interface{}
				s.session.On("Query", mock.AnythingOfType("string"), emptyArgs).Return(query)

				err := s.storage.Write("service-a")
				assert.NoError(t, err)
				err = s.storage.Write("service-b")
				assert.EqualError(t, err, "failed to Exec query 'select from service_names': exec error")
				assert.Equal(t, "[E] Failed to exec query query=select from service_names error=exec error\n", s.logBuffer.String())

				counts, _ := s.metricsFactory.Snapshot()
				assert.Equal(t, map[string]int64{
					"ServiceNames.attempts": 2, "ServiceNames.inserts": 1, "ServiceNames.errors": 1,
				}, counts)

				// write again
				err = s.storage.Write("service-a")
				assert.NoError(t, err)

				counts2, _ := s.metricsFactory.Snapshot()
				expCounts := counts
				if writeCacheTTL == 0 {
					// without write cache, the second write must succeed
					expCounts["ServiceNames.attempts"]++
					expCounts["ServiceNames.inserts"]++
				}
				assert.Equal(t, expCounts, counts2)
			})
		})
	}
}

func TestServiceNamesStorageGetServices(t *testing.T) {
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
		withServiceNamesStorage(writeCacheTTL, func(s *serviceNameStorageTest) {
			iter := &mocks.Iterator{}
			iter.On("Scan", matchOnce).Return(true)
			iter.On("Scan", matchEverything).Return(false) // false to stop the loop
			iter.On("Close").Return(expErr)

			query := &mocks.Query{}
			query.On("Iter").Return(iter)

			var emptyArgs []interface{}
			s.session.On("Query", mock.AnythingOfType("string"), emptyArgs).Return(query)

			services, err := s.storage.GetServices()
			if expErr == nil {
				assert.NoError(t, err)
				// expect empty string because mock iter.Scan(&placeholder) does not write to `placeholder`
				assert.Equal(t, []string{""}, services)
			} else {
				assert.EqualError(t, err, "Error reading service_names from storage: "+expErr.Error())
			}
		})

	}

}
