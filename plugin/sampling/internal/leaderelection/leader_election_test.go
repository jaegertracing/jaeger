// Copyright (c) 2018 The Jaeger Authors.
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

package leaderelection

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/atomic"

	lmocks "github.com/jaegertracing/jaeger/pkg/distributedlock/mocks"
	jio "github.com/jaegertracing/jaeger/pkg/io"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

var (
	errTestLock = errors.New("Lock error")
)

var _ io.Closer = &electionParticipant{}

func TestAcquireLock(t *testing.T) {
	leaderInterval := time.Millisecond
	followerInterval := 5 * time.Millisecond
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
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			logger, logBuffer := testutils.NewLogger()
			mockLock := &lmocks.Lock{}
			mockLock.On("Acquire", "sampling_lock", mock.AnythingOfType("time.Duration")).Return(test.acquiredLock, test.err)

			p := &electionParticipant{
				ElectionParticipantOptions: ElectionParticipantOptions{
					LeaderLeaseRefreshInterval:   leaderInterval,
					FollowerLeaseRefreshInterval: followerInterval,
					Logger:                       logger,
				},
				lock:         mockLock,
				resourceName: "sampling_lock",
				isLeader:     atomic.NewBool(false),
			}

			p.setLeader(test.isLeader)
			assert.Equal(t, test.expectedInterval, p.acquireLock())
			assert.Equal(t, test.expectedError, testutils.LogMatcher(1, acquireLockErrMsg, logBuffer.Lines()))
		})
	}
}

func TestRunAcquireLockLoop_followerOnly(t *testing.T) {
	logger, logBuffer := testutils.NewLogger()
	mockLock := &lmocks.Lock{}
	mockLock.On("Acquire", "sampling_lock", mock.AnythingOfType("time.Duration")).Return(false, errTestLock)

	p := NewElectionParticipant(mockLock, "sampling_lock", ElectionParticipantOptions{
		LeaderLeaseRefreshInterval:   time.Millisecond,
		FollowerLeaseRefreshInterval: 5 * time.Millisecond,
		Logger:                       logger,
	},
	)

	defer func() {
		assert.NoError(t, p.(io.Closer).Close())
	}()
	go p.(jio.Starter).Start()

	expectedErrorMsg := "Failed to acquire lock"
	for i := 0; i < 1000; i++ {
		// match logs specific to acquireLockErrMsg.
		if testutils.LogMatcher(2, expectedErrorMsg, logBuffer.Lines()) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	assert.True(t, testutils.LogMatcher(2, expectedErrorMsg, logBuffer.Lines()))
	assert.False(t, p.IsLeader())
}
