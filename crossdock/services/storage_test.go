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

package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/testutils"
)

func TestNewCassandraCluster(t *testing.T) {
	_, err := newCassandraCluster("localhost:8080", 4)
	assert.Error(t, err)
}

type storageTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
}

func withStorage(fn func(s *storageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	s := &storageTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
	}
	fn(s)
}

func TestInitializeCassandraSchema(t *testing.T) {
	testCases := []struct {
		caption       string
		queryError    error
		expectedError string
	}{
		{
			caption: "success",
		},
		{
			caption:       "failure",
			queryError:    errors.New("query error"),
			expectedError: "Error reading throughput from storage: query error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withStorage(func(s *storageTest) {
				query := &mocks.Query{}
				query.On("Exec").Return(testCase.queryError)
				session := &mocks.Session{}
				session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query).Times(2)

				initializeCassandraSchema(s.logger, "fixtures/test-schema.cql", "", session)
				if testCase.expectedError == "" {
					assert.Equal(t, "", s.logBuffer.String())
				} else {
					assert.Contains(t, s.logBuffer.String(), "CREATE KEYSPACE IF NOT EXISTS jaeger;")
				}
			})
		})
	}
}

func matchEverything() interface{} {
	return mock.MatchedBy(func(v []interface{}) bool { return true })
}
