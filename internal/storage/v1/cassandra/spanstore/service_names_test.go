// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type serviceNameStorageTest struct {
	session        *mocks.Session
	writeCacheTTL  time.Duration
	metricsFactory *metricstest.Factory
	logger         *zap.Logger
	logBuffer      *testutils.Buffer
	storage        *ServiceNamesStorage
}

func withServiceNamesStorage(writeCacheTTL time.Duration, fn func(s *serviceNameStorageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(time.Second)
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
				execError := errors.New("exec error")
				query := &mocks.Query{}
				query1 := &mocks.Query{}
				query2 := &mocks.Query{}
				query.On("Bind", []any{"service-a"}).Return(query1)
				query.On("Bind", []any{"service-b"}).Return(query2)
				query1.On("Exec").Return(nil)
				query2.On("Exec").Return(execError)
				query2.On("String").Return("select from service_names")

				s.session.On("Query", mock.AnythingOfType("string")).Return(query)

				err := s.storage.Write("service-a")
				require.NoError(t, err)
				err = s.storage.Write("service-b")
				require.EqualError(t, err, "failed to Exec query 'select from service_names': exec error")
				assert.Equal(t, map[string]string{
					"level": "error",
					"msg":   "Failed to exec query",
					"query": "select from service_names",
					"error": "exec error",
				}, s.logBuffer.JSONLine(0))

				counts, _ := s.metricsFactory.Snapshot()
				assert.Equal(t, map[string]int64{
					"attempts|table=service_names": 2, "inserts|table=service_names": 1, "errors|table=service_names": 1,
				}, counts)

				// write again
				err = s.storage.Write("service-a")
				require.NoError(t, err)

				counts2, _ := s.metricsFactory.Snapshot()
				expCounts := counts
				if writeCacheTTL == 0 {
					// without write cache, the second write must succeed
					expCounts["attempts|table=service_names"]++
					expCounts["inserts|table=service_names"]++
				}
				assert.Equal(t, expCounts, counts2)
			})
		})
	}
}

func TestServiceNamesStorageGetServices(t *testing.T) {
	scanError := errors.New("scan error")
	var writeCacheTTL time.Duration
	var matched bool
	matchOnce := mock.MatchedBy(func(_ []any) bool {
		if matched {
			return false
		}
		matched = true
		return true
	})
	matchEverything := mock.MatchedBy(func(_ []any) bool { return true })
	for _, expErr := range []error{nil, scanError} {
		withServiceNamesStorage(writeCacheTTL, func(s *serviceNameStorageTest) {
			iter := &mocks.Iterator{}
			iter.On("Scan", matchOnce).Return(true)
			iter.On("Scan", matchEverything).Return(false) // false to stop the loop
			iter.On("Close").Return(expErr)

			query := &mocks.Query{}
			query.On("Iter").Return(iter)

			s.session.On("Query", mock.AnythingOfType("string")).Return(query)

			services, err := s.storage.GetServices()
			if expErr == nil {
				require.NoError(t, err)
				// expect empty string because mock iter.Scan(&placeholder) does not write to `placeholder`
				assert.Equal(t, []string{""}, services)
			} else {
				require.EqualError(t, err, "error reading service_names from storage: "+expErr.Error())
			}
		})
	}
}
