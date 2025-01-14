// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package leaderelection

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	dl "github.com/jaegertracing/jaeger/pkg/distributedlock"
)

const (
	acquireLockErrMsg = "Failed to acquire lock"
)

// ElectionParticipant partakes in leader election to become leader.
type ElectionParticipant interface {
	io.Closer

	IsLeader() bool
	Start() error
}

// DistributedElectionParticipant implements ElectionParticipant on top of a distributed lock.
type DistributedElectionParticipant struct {
	ElectionParticipantOptions
	lock         dl.Lock
	isLeader     atomic.Bool
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
func NewElectionParticipant(lock dl.Lock, resourceName string, options ElectionParticipantOptions) *DistributedElectionParticipant {
	return &DistributedElectionParticipant{
		ElectionParticipantOptions: options,
		lock:                       lock,
		resourceName:               resourceName,
		closeChan:                  make(chan struct{}),
	}
}

// Start runs a background thread which attempts to acquire the leader lock.
func (p *DistributedElectionParticipant) Start() error {
	p.wg.Add(1)
	go p.runAcquireLockLoop()
	return nil
}

// Close implements io.Closer.
func (p *DistributedElectionParticipant) Close() error {
	close(p.closeChan)
	p.wg.Wait()
	return nil
}

// IsLeader returns true if this process is the leader.
func (p *DistributedElectionParticipant) IsLeader() bool {
	return p.isLeader.Load()
}

// runAcquireLockLoop attempts to acquire the leader lock. If it succeeds, it will attempt to retain it,
// otherwise it sleeps and attempts to gain the lock again.
func (p *DistributedElectionParticipant) runAcquireLockLoop() {
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
func (p *DistributedElectionParticipant) acquireLock() time.Duration {
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

func (p *DistributedElectionParticipant) setLeader(isLeader bool) {
	p.isLeader.Store(isLeader)
}
