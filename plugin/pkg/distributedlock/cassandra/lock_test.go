// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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
			expectedErrMsg: "this host does not own the resource lock",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withCQLLock(func(s *cqlLockTest) {
				query := &mocks.Query{}
				query.On("ScanCAS", mock.Anything).Return(testCase.applied, testCase.errScan)

				s.session.On(
					"Query",
					mock.AnythingOfType("string"),
					[]any{
						60,
						localhost,
						samplingLock,
						localhost,
					},
				).Return(query)
				err := s.lock.extendLease(samplingLock, time.Second*60)
				if testCase.expectedErrMsg == "" {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, testCase.expectedErrMsg)
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
			expectedErrMsg:    "failed to acquire resource lock due to cassandra error: Failed to create lock",
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
			expectedErrMsg:    "failed to extend lease on resource lock: this host does not own the resource lock",
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
				assignPtr := func(vals ...string) any {
					return mock.MatchedBy(func(args []any) bool {
						if len(args) != len(vals) {
							return false
						}
						for i, arg := range args {
							ptr, ok := arg.(*string)
							if !ok {
								return false
							}
							*ptr = vals[i]
						}
						return true
					})
				}
				firstQuery := &mocks.Query{}
				firstQuery.On("ScanCAS", assignPtr(testCase.retVals...)).
					Return(testCase.insertLockApplied, testCase.errScan)
				secondQuery := &mocks.Query{}
				secondQuery.On("ScanCAS", mock.Anything).Return(testCase.updateLockApplied, nil)

				s.session.On("Query", stringMatcher("INSERT INTO leases"), mock.Anything).Return(firstQuery)
				s.session.On("Query", stringMatcher("UPDATE leases"), mock.Anything).Return(secondQuery)
				acquired, err := s.lock.Acquire(samplingLock, 0)
				if testCase.expectedErrMsg == "" {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, testCase.expectedErrMsg)
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
			expectedErrMsg: "failed to forfeit resource lock due to cassandra error: Failed to delete lock",
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
			expectedErrMsg: "failed to forfeit resource lock: this host does not own the resource lock",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withCQLLock(func(s *cqlLockTest) {
				query := &mocks.Query{}
				query.On("ScanCAS", mock.Anything).Return(testCase.applied, testCase.errScan)

				s.session.On("Query", mock.AnythingOfType("string"), []any{samplingLock, localhost}).Return(query)
				applied, err := s.lock.Forfeit(samplingLock)
				if testCase.expectedErrMsg == "" {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, testCase.expectedErrMsg)
				}
				assert.Equal(t, testCase.applied, applied)
			})
		})
	}
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) any {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
