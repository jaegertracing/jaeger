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

package cassandra

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
)

var (
	localhost    = "localhost"
	samplingLock = "sampling_lock"
)

type cqlLockTest struct {
	session *mocks.Session
	lock    *Lock
}

func withCQLLock(fn func(r *cqlLockTest)) {
	session := &mocks.Session{}
	r := &cqlLockTest{
		session: session,
		lock:    NewLock(session, localhost),
	}
	fn(r)
}

func TestExtendLease(t *testing.T) {
	testCases := []struct {
		caption        string
		applied        bool
		errScan        error
		expectedErrMsg string
	}{
		{
			caption:        "cassandra error",
			applied:        false,
			errScan:        errors.New("Failed to update lock"),
			expectedErrMsg: "Failed to update lock",
		},
		{
			caption:        "successfully extended lease",
			applied:        true,
			errScan:        nil,
			expectedErrMsg: "",
		},
		{
			caption:        "failed to extend lease",
			applied:        false,
			errScan:        nil,
			expectedErrMsg: "This host does not own the resource lock",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withCQLLock(func(s *cqlLockTest) {
				query := &mocks.Query{}
				query.On("ScanCAS", matchEverything()).Return(testCase.applied, testCase.errScan)

				var args []interface{}
				captureArgs := mock.MatchedBy(func(v []interface{}) bool {
					args = v
					return true
				})

				s.session.On("Query", mock.AnythingOfType("string"), captureArgs).Return(query)
				err := s.lock.extendLease(samplingLock, time.Second*60)
				if testCase.expectedErrMsg == "" {
					assert.NoError(t, err)
				} else {
					assert.EqualError(t, err, testCase.expectedErrMsg)
				}

				assert.Len(t, args, 4)
				if d, ok := args[0].(float64); ok {
					assert.EqualValues(t, 60, d)
				} else {
					assert.Fail(t, "expecting first arg as int64", "received: %+v", args)
				}
				if d, ok := args[1].(string); ok {
					assert.Equal(t, localhost, d)
				} else {
					assert.Fail(t, "expecting second arg as string", "received: %+v", args)
				}
				if d, ok := args[2].(string); ok {
					assert.Equal(t, samplingLock, d)
				} else {
					assert.Fail(t, "expecting third arg as string", "received: %+v", args)
				}
				if d, ok := args[3].(string); ok {
					assert.Equal(t, localhost, d)
				} else {
					assert.Fail(t, "expecting fourth arg as string", "received: %+v", args)
				}
			})
		})
	}
}

func TestAcquire(t *testing.T) {
	testCases := []struct {
		caption           string
		insertLockApplied bool
		retVals           []string
		acquired          bool
		updateLockApplied bool
		errScan           error
		expectedErrMsg    string
	}{
		{
			caption:           "cassandra error",
			insertLockApplied: false,
			retVals:           []string{"", ""},
			acquired:          false,
			errScan:           errors.New("Failed to create lock"),
			expectedErrMsg:    "Failed to acquire resource lock due to cassandra error: Failed to create lock",
		},
		{
			caption:           "successfully created lock",
			insertLockApplied: true,
			acquired:          true,
			retVals:           []string{samplingLock, localhost},
			errScan:           nil,
			expectedErrMsg:    "",
		},
		{
			caption:           "lock already exists and belongs to localhost",
			insertLockApplied: false,
			acquired:          true,
			retVals:           []string{samplingLock, localhost},
			updateLockApplied: true,
			errScan:           nil,
			expectedErrMsg:    "",
		},
		{
			caption:           "lock already exists and belongs to localhost but is lost",
			insertLockApplied: false,
			acquired:          false,
			retVals:           []string{samplingLock, localhost},
			updateLockApplied: false,
			errScan:           nil,
			expectedErrMsg:    "Failed to extend lease on resource lock: This host does not own the resource lock",
		},
		{
			caption:           "failed to acquire lock",
			insertLockApplied: false,
			acquired:          false,
			retVals:           []string{samplingLock, "otherhost"},
			errScan:           nil,
			expectedErrMsg:    "",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withCQLLock(func(s *cqlLockTest) {
				firstQuery := &mocks.Query{}
				secondQuery := &mocks.Query{}

				scanMatcher := func() interface{} {
					scanFunc := func(args []interface{}) bool {
						for i, arg := range args {
							if ptr, ok := arg.(*string); ok {
								*ptr = testCase.retVals[i]
							}
						}
						return true
					}
					return mock.MatchedBy(scanFunc)
				}
				firstQuery.On("ScanCAS", scanMatcher()).Return(testCase.insertLockApplied, testCase.errScan)
				secondQuery.On("ScanCAS", matchEverything()).Return(testCase.updateLockApplied, nil)

				s.session.On("Query", stringMatcher("INSERT INTO leases"), matchEverything()).Return(firstQuery)
				s.session.On("Query", stringMatcher("UPDATE leases"), matchEverything()).Return(secondQuery)
				acquired, err := s.lock.Acquire(samplingLock, 0)
				if testCase.expectedErrMsg == "" {
					assert.NoError(t, err)
				} else {
					assert.EqualError(t, err, testCase.expectedErrMsg)
				}

				assert.Equal(t, testCase.acquired, acquired)
			})
		})
	}
}

func TestForfeit(t *testing.T) {
	testCases := []struct {
		caption        string
		applied        bool
		retVals        []string
		errScan        error
		expectedErrMsg string
	}{
		{
			caption:        "cassandra error",
			applied:        false,
			retVals:        []string{"", ""},
			errScan:        errors.New("Failed to delete lock"),
			expectedErrMsg: "Failed to forfeit resource lock due to cassandra error: Failed to delete lock",
		},
		{
			caption:        "successfully forfeited lock",
			applied:        true,
			retVals:        []string{samplingLock, localhost},
			errScan:        nil,
			expectedErrMsg: "",
		},
		{
			caption:        "failed to delete lock",
			applied:        false,
			retVals:        []string{samplingLock, "otherhost"},
			errScan:        nil,
			expectedErrMsg: "Failed to forfeit resource lock: This host does not own the resource lock",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withCQLLock(func(s *cqlLockTest) {
				query := &mocks.Query{}
				query.On("ScanCAS", matchEverything()).Return(testCase.applied, testCase.errScan)

				var args []interface{}
				captureArgs := mock.MatchedBy(func(v []interface{}) bool {
					args = v
					return true
				})

				s.session.On("Query", mock.AnythingOfType("string"), captureArgs).Return(query)
				applied, err := s.lock.Forfeit(samplingLock)
				if testCase.expectedErrMsg == "" {
					assert.NoError(t, err)
				} else {
					assert.EqualError(t, err, testCase.expectedErrMsg)
				}
				assert.Equal(t, testCase.applied, applied)

				assert.Len(t, args, 2)
				if d, ok := args[0].(string); ok {
					assert.Equal(t, samplingLock, d)
				} else {
					assert.Fail(t, "expecting first arg as string", "received: %+v", args)
				}
				if d, ok := args[1].(string); ok {
					assert.Equal(t, localhost, d)
				} else {
					assert.Fail(t, "expecting second arg as string", "received: %+v", args)
				}
			})
		})
	}
}

func matchEverything() interface{} {
	return mock.MatchedBy(func(v []interface{}) bool { return true })
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
