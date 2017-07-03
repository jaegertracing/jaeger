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
)

type storageTest struct {
	session *mocks.Session
}

func withStorage(fn func(s *storageTest)) {
	session := &mocks.Session{}
	s := &storageTest{
		session: session,
	}
	fn(s)
}

func TestInitializeCassandraSchema(t *testing.T) {
	schemaFile := "fixtures/test-schema.cql"

	testCases := []struct {
		caption       string
		schemaFile    string
		queryError    error
		expectedError string
	}{
		{
			caption:    "success",
			schemaFile: schemaFile,
		},
		{
			caption:       "query error",
			schemaFile:    schemaFile,
			queryError:    errors.New("query error"),
			expectedError: "Failed to apply a schema query: DROP KEYSPACE IF EXISTS: query error",
		},
		{
			caption:       "schema error",
			expectedError: "open : no such file or directory",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withStorage(func(s *storageTest) {
				query := &mocks.Query{}
				query.On("Exec").Return(testCase.queryError)
				session := &mocks.Session{}
				session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

				err := InitializeCassandraSchema(session, testCase.schemaFile, "", zap.NewNop())
				if testCase.expectedError == "" {
					assert.NoError(t, err)
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}
			})
		})
	}
}

func matchEverything() interface{} {
	return mock.MatchedBy(func(v []interface{}) bool { return true })
}
