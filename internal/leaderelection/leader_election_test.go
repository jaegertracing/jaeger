// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package leaderelection

import (
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lmocks "github.com/jaegertracing/jaeger/internal/distributedlock/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var errTestLock = errors.New("lock error")

var _ io.Closer = new(DistributedElectionParticipant)

func TestAcquireLock(t *testing.T) {
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)
	tests := []struct {
		isLeader         bool
		acquiredLock     bool
		err              error
		expectedInterval time.Duration
		expectedError    bool
	}{
		{isLeader: true, acquiredLock: true, err: nil, expectedInterval: leaderInterval, expectedError: false},
		{isLeader: true, acquiredLock: false, err: errTestLock, expectedInterval: leaderInterval, expectedError: true},
		{isLeader: true, acquiredLock: false, err: nil, expectedInterval: followerInterval, expectedError: false},
		{isLeader: false, acquiredLock: false, err: nil, expectedInterval: followerInterval, expectedError: false},
		{isLeader: false, acquiredLock: false, err: errTestLock, expectedInterval: followerInterval, expectedError: true},
		{isLeader: false, acquiredLock: true, err: nil, expectedInterval: leaderInterval, expectedError: false},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			logger, logBuffer := testutils.NewLogger()
			mockLock := &lmocks.Lock{}
			mockLock.On("Acquire", "sampling_lock", followerInterval).Return(test.acquiredLock, test.err)

			p := &DistributedElectionParticipant{
				ElectionParticipantOptions: ElectionParticipantOptions{
					LeaderLeaseRefreshInterval:   leaderInterval,
					FollowerLeaseRefreshInterval: followerInterval,
					Logger:                       logger,
				},
				lock:         mockLock,
				resourceName: "sampling_lock",
			}

			p.setLeader(test.isLeader)
			assert.Equal(t, test.expectedInterval, p.acquireLock())
			match, errMsg := testutils.LogMatcher(1, acquireLockErrMsg, logBuffer.Lines())
			assert.Equal(t, test.expectedError, match, errMsg)
		})
	}
}

func TestRunAcquireLockLoopFollowerOnly(t *testing.T) {
	logger, logBuffer := testutils.NewLogger()
	mockLock := &lmocks.Lock{}
	mockLock.On("Acquire", "sampling_lock", time.Duration(5*time.Millisecond)).Return(false, errTestLock)

	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   time.Millisecond,
			FollowerLeaseRefreshInterval: 5 * time.Millisecond,
			Logger:                       logger,
		},
	)

	defer func() {
		require.NoError(t, p.Close())
	}()
	go p.Start()

	expectedErrorMsg := "Failed to acquire lock"
	for range 1000 {
		// match logs specific to acquireLockErrMsg.
		if match, _ := testutils.LogMatcher(2, expectedErrorMsg, logBuffer.Lines()); match {
			break
		}
		time.Sleep(time.Millisecond)
	}
	match, errMsg := testutils.LogMatcher(2, expectedErrorMsg, logBuffer.Lines())
	assert.True(t, match, errMsg)
	assert.False(t, p.IsLeader())
}

func TestNewElectionParticipantWithInvalidDurations(t *testing.T) {
	tests := []struct {
		name                     string
		leaderInterval           time.Duration
		followerInterval         time.Duration
		expectedLeaderInterval   time.Duration
		expectedFollowerInterval time.Duration
	}{
		{
			name:                     "zero leader and follower intervals",
			leaderInterval:           0,
			followerInterval:         0,
			expectedLeaderInterval:   defaultLeaderLeaseRefreshInterval,
			expectedFollowerInterval: defaultFollowerLeaseRefreshInterval,
		},
		{
			name:                     "negative leader interval",
			leaderInterval:           -1 * time.Second,
			followerInterval:         5 * time.Millisecond,
			expectedLeaderInterval:   defaultLeaderLeaseRefreshInterval,
			expectedFollowerInterval: 5 * time.Millisecond,
		},
		{
			name:                     "negative follower interval",
			leaderInterval:           1 * time.Millisecond,
			followerInterval:         -10 * time.Second,
			expectedLeaderInterval:   1 * time.Millisecond,
			expectedFollowerInterval: defaultFollowerLeaseRefreshInterval,
		},
		{
			name:                     "both negative intervals",
			leaderInterval:           -5 * time.Second,
			followerInterval:         -10 * time.Second,
			expectedLeaderInterval:   defaultLeaderLeaseRefreshInterval,
			expectedFollowerInterval: defaultFollowerLeaseRefreshInterval,
		},
		{
			name:                     "valid positive intervals unchanged",
			leaderInterval:           2 * time.Second,
			followerInterval:         8 * time.Second,
			expectedLeaderInterval:   2 * time.Second,
			expectedFollowerInterval: 8 * time.Second,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testutils.NewLogger()
			mockLock := &lmocks.Lock{}

			p := NewElectionParticipant(
				mockLock, "test_lock", ElectionParticipantOptions{
					LeaderLeaseRefreshInterval:   test.leaderInterval,
					FollowerLeaseRefreshInterval: test.followerInterval,
					Logger:                       logger,
				},
			)

			assert.Equal(t, test.expectedLeaderInterval, p.LeaderLeaseRefreshInterval)
			assert.Equal(t, test.expectedFollowerInterval, p.FollowerLeaseRefreshInterval)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
