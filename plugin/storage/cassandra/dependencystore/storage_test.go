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

package dependencystore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/storage/dependencystore"
)

type depStorageTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStore(fn func(s *depStorageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metrics.NewLocalFactory(time.Second)
	defer metricsFactory.Stop()
	s := &depStorageTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
		storage:   NewDependencyStore(session, 24*time.Hour, metricsFactory, logger),
	}
	fn(s)
}

func TestNewDependencyStore(t *testing.T) {
	withDepStore(func(s *depStorageTest) {
		var reader dependencystore.Reader = s.storage // check API conformance
		var writer dependencystore.Writer = s.storage // check API conformance
		assert.NotNil(t, reader)
		assert.NotNil(t, writer)
	})
}

func TestDependencyStoreWrite(t *testing.T) {
	withDepStore(func(s *depStorageTest) {
		query := &mocks.Query{}
		query.On("Exec").Return(nil)

		var args []interface{}
		captureArgs := mock.MatchedBy(func(v []interface{}) bool {
			args = v
			return true
		})

		s.session.On("Query", mock.AnythingOfType("string"), captureArgs).Return(query)

		ts := time.Date(2017, time.January, 24, 11, 15, 17, 12345, time.UTC)
		dependencies := []model.DependencyLink{
			{
				Parent:    "a",
				Child:     "b",
				CallCount: 42,
			},
		}
		err := s.storage.WriteDependencies(ts, dependencies)
		assert.NoError(t, err)

		assert.Len(t, args, 3)
		if d, ok := args[0].(time.Time); ok {
			assert.Equal(t, ts, d)
		} else {
			assert.Fail(t, "expecting first arg as time.Time", "received: %+v", args)
		}
		if d, ok := args[1].(time.Time); ok {
			assert.Equal(t, ts, d)
		} else {
			assert.Fail(t, "expecting second arg as time.Time", "received: %+v", args)
		}
		if d, ok := args[2].([]Dependency); ok {
			assert.Equal(t, []Dependency{
				{
					Parent:    "a",
					Child:     "b",
					CallCount: 42,
				},
			}, d)
		} else {
			assert.Fail(t, "expecting third arg as []Dependency", "received: %+v", args)
		}
	})
}

func TestDependencyStoreGetDependencies(t *testing.T) {
	testCases := []struct {
		caption       string
		queryError    error
		expectedError string
		expectedLogs  []string
	}{
		{
			caption: "success",
		},
		{
			caption:       "failure",
			queryError:    errors.New("query error"),
			expectedError: "Error reading dependencies from storage: query error",
			expectedLogs: []string{
				"Failure to read Dependencies",
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withDepStore(func(s *depStorageTest) {
				scanMatcher := func() interface{} {
					deps := [][]Dependency{
						{
							{Parent: "a", Child: "b", CallCount: 1},
							{Parent: "b", Child: "c", CallCount: 1},
						},
						{
							{Parent: "a", Child: "b", CallCount: 1},
							{Parent: "b", Child: "c", CallCount: 1},
						},
					}
					scanFunc := func(args []interface{}) bool {
						if len(deps) == 0 {
							return false
						}
						for _, arg := range args {
							if ptr, ok := arg.(*[]Dependency); ok {
								*ptr = deps[0]
								break
							}
						}
						deps = deps[1:]
						return true
					}
					return mock.MatchedBy(scanFunc)
				}

				iter := &mocks.Iterator{}
				iter.On("Scan", scanMatcher()).Return(true)
				iter.On("Scan", matchEverything()).Return(false)
				iter.On("Close").Return(testCase.queryError)

				query := &mocks.Query{}
				query.On("Exec").Return(nil)
				query.On("Consistency", cassandra.One).Return(query)
				query.On("Iter").Return(iter)

				s.session.On("Query", mock.AnythingOfType("string"), matchEverything()).Return(query)

				deps, err := s.storage.GetDependencies(time.Now(), 48*time.Hour)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					expected := []model.DependencyLink{
						{Parent: "a", Child: "b", CallCount: 1},
						{Parent: "b", Child: "c", CallCount: 1},
						{Parent: "a", Child: "b", CallCount: 1},
						{Parent: "b", Child: "c", CallCount: 1},
					}
					assert.Equal(t, expected, deps)
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}
				for _, expectedLog := range testCase.expectedLogs {
					assert.True(t, strings.Contains(s.logBuffer.String(), expectedLog), "Log must contain %s, but was %s", expectedLog, s.logBuffer.String())
				}
				if len(testCase.expectedLogs) == 0 {
					assert.Equal(t, "", s.logBuffer.String())
				}
			})
		})
	}
}

func TestDependencyStoreTimeIntervalToPoints(t *testing.T) {
	withDepStore(func(s *depStorageTest) {
		for _, truncate := range []bool{false, true} {
			endTs := time.Date(2017, time.January, 24, 11, 15, 17, 12345, time.UTC)
			if truncate {
				endTs = endTs.Truncate(s.storage.dependencyDataFrequency)
			}
			// Look back 3 time intervals
			lookback := 3 * s.storage.dependencyDataFrequency
			points := s.storage.timeIntervalToPoints(endTs, lookback)
			point := func(day int) time.Time {
				return time.Date(2017, time.January, day, 0, 0, 0, 0, time.UTC)
			}
			assert.Equal(t, []time.Time{
				point(24),
				point(23),
				point(22),
			}, points, "Expecting 3 data points")
		}
	})
}

func matchEverything() interface{} {
	return mock.MatchedBy(func(v []interface{}) bool { return true })
}
