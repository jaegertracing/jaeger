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
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"

	dl "github.com/jaegertracing/jaeger/pkg/distributedlock"
)

var (
	acquireLockErrMsg = "Failed to acquire lock"
)

// ElectionParticipant partakes in leader election to become leader.
type ElectionParticipant interface {
	IsLeader() bool
}

type electionParticipant struct {
	ElectionParticipantOptions
	lock         dl.Lock
	isLeader     *atomic.Bool
	resourceName string
	closeChan    chan struct{}
	wg           sync.WaitGroup
}

// ElectionParticipantOptions control behavior of the election participant. TODO func applyDefaults(), parameter error checking, etc.
type ElectionParticipantOptions struct {
	LeaderLeaseRefreshInterval   time.Duration
	FollowerLeaseRefreshInterval time.Duration
	Logger                       *zap.Logger
}

// NewElectionParticipant returns a ElectionParticipant which attempts to become leader.
func NewElectionParticipant(lock dl.Lock, resourceName string, options ElectionParticipantOptions) ElectionParticipant {
	return &electionParticipant{
		ElectionParticipantOptions: options,
		lock:                       lock,
		resourceName:               resourceName,
		isLeader:                   atomic.NewBool(false),
		closeChan:                  make(chan struct{}),
	}
}

// Start runs a background thread which attempts to acquire the leader lock.
func (p *electionParticipant) Start() error {
	p.wg.Add(1)
	go p.runAcquireLockLoop()
	return nil
}

// Close implements io.Closer.
func (p *electionParticipant) Close() error {
	close(p.closeChan)
	p.wg.Wait()
	return nil
}

// IsLeader returns true if this process is the leader.
func (p *electionParticipant) IsLeader() bool {
	return p.isLeader.Load()
}

// runAcquireLockLoop attempts to acquire the leader lock. If it succeeds, it will attempt to retain it,
// otherwise it sleeps and attempts to gain the lock again.
func (p *electionParticipant) runAcquireLockLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.acquireLock())
	for {
		select {
		case <-ticker.C:
			ticker.Stop()
			ticker = time.NewTicker(p.acquireLock())
		case <-p.closeChan:
			ticker.Stop()
			return
		}
	}
}

// acquireLock attempts to acquire the lock and returns the interval to sleep before the next retry.
func (p *electionParticipant) acquireLock() time.Duration {
	if acquiredLeaderLock, err := p.lock.Acquire(p.resourceName, p.FollowerLeaseRefreshInterval); err == nil {
		p.setLeader(acquiredLeaderLock)
	} else {
		p.Logger.Error(acquireLockErrMsg, zap.Error(err))
	}
	if p.IsLeader() {
		// If this process holds the leader lock, retry with a shorter cadence
		// to retain the leader lease.
		return p.LeaderLeaseRefreshInterval
	}
	// If this process failed to acquire the leader lock, retry with a longer cadence
	return p.FollowerLeaseRefreshInterval
}

func (p *electionParticipant) setLeader(isLeader bool) {
	p.isLeader.Store(isLeader)
}
