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

package dependencystore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

type depStorageTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withDepStore(version Version, fn func(s *depStorageTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(time.Second)
	defer metricsFactory.Stop()
	store, _ := NewDependencyStore(session, metricsFactory, logger, version)
	s := &depStorageTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
		storage:   store,
	}
	fn(s)
}

var _ dependencystore.Reader = &DependencyStore{} // check API conformance
var _ dependencystore.Writer = &DependencyStore{} // check API conformance

func TestVersionIsValid(t *testing.T) {
	assert.True(t, V1.IsValid())
	assert.True(t, V2.IsValid())
	assert.False(t, versionEnumEnd.IsValid())
}

func TestInvalidVersion(t *testing.T) {
	_, err := NewDependencyStore(&mocks.Session{}, metrics.NullFactory, zap.NewNop(), versionEnumEnd)
	assert.Error(t, err)
}

func TestDependencyStoreWrite(t *testing.T) {
	testCases := []struct {
		caption string
		version Version
	}{
		{
			caption: "V1",
			version: V1,
		},
		{
			caption: "V2",
			version: V2,
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withDepStore(testCase.version, func(s *depStorageTest) {
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
						Source:    model.JaegerDependencyLinkSource,
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
				if testCase.version == V2 {
					if d, ok := args[1].(time.Time); ok {
						assert.Equal(t, time.Date(2017, time.January, 24, 0, 0, 0, 0, time.UTC), d)
					} else {
						assert.Fail(t, "expecting second arg as time", "received: %+v", args)
					}
				} else {
					if d, ok := args[1].(time.Time); ok {
						assert.Equal(t, ts, d)
					} else {
						assert.Fail(t, "expecting second arg as time.Time", "received: %+v", args)
					}
				}
				if d, ok := args[2].([]Dependency); ok {
					assert.Equal(t, []Dependency{
						{
							Parent:    "a",
							Child:     "b",
							CallCount: 42,
							Source:    "jaeger",
						},
					}, d)
				} else {
					assert.Fail(t, "expecting third arg as []Dependency", "received: %+v", args)
				}
			})
		})
	}
}

func TestDependencyStoreGetDependencies(t *testing.T) {
	testCases := []struct {
		caption       string
		queryError    error
		expectedError string
		expectedLogs  []string
		version       Version
	}{
		{
			caption: "success V1",
			version: V1,
		},
		{
			caption: "success V2",
			version: V2,
		},
		{
			caption:       "failure V1",
			queryError:    errors.New("query error"),
			expectedError: "error reading dependencies from storage: query error",
			expectedLogs: []string{
				"Failure to read Dependencies",
			},
			version: V1,
		},
		{
			caption:       "failure V2",
			queryError:    errors.New("query error"),
			expectedError: "error reading dependencies from storage: query error",
			expectedLogs: []string{
				"Failure to read Dependencies",
			},
			version: V2,
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withDepStore(testCase.version, func(s *depStorageTest) {
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

				deps, err := s.storage.GetDependencies(context.Background(), time.Now(), 48*time.Hour)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					expected := []model.DependencyLink{
						{Parent: "a", Child: "b", CallCount: 1, Source: model.JaegerDependencyLinkSource},
						{Parent: "b", Child: "c", CallCount: 1, Source: model.JaegerDependencyLinkSource},
						{Parent: "a", Child: "b", CallCount: 1, Source: model.JaegerDependencyLinkSource},
						{Parent: "b", Child: "c", CallCount: 1, Source: model.JaegerDependencyLinkSource},
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

func TestGetBuckets(t *testing.T) {
	var (
		start    = time.Date(2017, time.January, 24, 11, 15, 17, 12345, time.UTC)
		end      = time.Date(2017, time.January, 26, 11, 15, 17, 12345, time.UTC)
		expected = []time.Time{
			time.Date(2017, time.January, 24, 0, 0, 0, 0, time.UTC),
			time.Date(2017, time.January, 25, 0, 0, 0, 0, time.UTC),
			time.Date(2017, time.January, 26, 0, 0, 0, 0, time.UTC),
		}
	)
	assert.Equal(t, expected, getBuckets(start, end))
}

func matchEverything() interface{} {
	return mock.MatchedBy(func(v []interface{}) bool { return true })
}
