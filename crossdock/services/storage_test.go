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
