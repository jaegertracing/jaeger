package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/uber/jaeger/pkg/testutils"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/cassandra/mocks"
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
