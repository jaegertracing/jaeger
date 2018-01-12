// Copyright (c) 2018 Uber Technologies, Inc.
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

package throttling

import (
	"io"
	"math"
	"sync"
	"time"
)

// tokenBucket is a simple token bucket used to refill credits for services
// with increasing time.
type tokenBucket struct {
	creditsPerSecond float64
	balance          float64
	maxBalance       float64
	lastTick         time.Time
	timeNow          func() time.Time // For testing
}

func newTokenBucket(options CreditAccruerOptions, timeNow func() time.Time) *tokenBucket {
	return &tokenBucket{
		creditsPerSecond: options.CreditsPerSecond,
		balance:          options.MaxBalance,
		maxBalance:       options.MaxBalance,
		timeNow:          timeNow,
		lastTick:         timeNow(),
	}
}

// Withdraw deducts as much of the total balance possible without exceeding
// maxWithdrawal.
func (t *tokenBucket) Withdraw(maxWithdrawal float64) float64 {
	now := t.timeNow()
	interval := now.Sub(t.lastTick)
	t.lastTick = now
	diff := interval.Seconds() * t.creditsPerSecond
	t.balance = math.Min(t.balance+diff, t.maxBalance)
	result := math.Min(t.balance, maxWithdrawal)
	t.balance -= result
	return result
}

// client represents a Jaeger-instrumented caller. Every Jaeger client
// corresponds to an instrumented service, but it may be one of many instances
// of that service. Therefore, there is a one-to-many relationship between
// services and clients. The clients must request credits from the service's
// creditAccruer and they all share the same underlying tokenBucket.
// For example, if one client requests a withdrawal and receives all the service
// credits, the next client withdrawal request will be rejected until the
// credits refill.
type client struct {
	// id is a unique id for the instance of the Jaeger client
	id string
	// perOperationBalance maintains per-operation balances of withdrawn credits
	perOperationBalance map[string]float64
	// updateTime is the last time client was updated
	updateTime time.Time
}

func newClient(id string, currentTime time.Time) *client {
	return &client{
		id:                  id,
		updateTime:          currentTime,
		perOperationBalance: map[string]float64{},
	}
}

// Spend depletes the balance from the client for the given operation. The usage
// would be when a client submits spans for operation X, it is spending those
// credits it requested for operation X.
func (c *client) Spend(operationName string, credits float64) bool {
	balance := c.perOperationBalance[operationName]
	if credits > balance {
		return false
	}

	balance -= credits
	c.perOperationBalance[operationName] = balance
	return true
}

// CreditAccruerOptions provides values to be used with a creditAccruer object
type CreditAccruerOptions struct {
	MaxOperations    int
	CreditsPerSecond float64
	MaxBalance       float64
	ClientMaxBalance float64
}

type creditAccruer struct {
	options                 CreditAccruerOptions
	serviceName             string
	perOperationRateLimiter OverrideMap
	updateTime              time.Time
	timeNow                 func() time.Time // For testing
}

func newCreditAccruer(serviceName string, options CreditAccruerOptions, timeNow func() time.Time) *creditAccruer {
	defaultRateLimiter := newTokenBucket(options, timeNow)
	perOperationRateLimiter := NewOverrideMap(options.MaxOperations, defaultRateLimiter)
	return &creditAccruer{
		options:                 options,
		serviceName:             serviceName,
		updateTime:              timeNow(),
		timeNow:                 timeNow,
		perOperationRateLimiter: perOperationRateLimiter,
	}
}

// Withdraw credits from the creditAccruer for the given operationName with an
// upper limit of maxWithdrawal.
func (accruer *creditAccruer) Withdraw(operationName string, maxWithdrawal float64) float64 {
	var rateLimiter *tokenBucket
	if accruer.perOperationRateLimiter.Has(operationName) ||
		accruer.perOperationRateLimiter.IsFull() {
		rateLimiter = accruer.perOperationRateLimiter.Get(operationName).(*tokenBucket)
	} else {
		rateLimiter = newTokenBucket(accruer.options, accruer.timeNow)
		accruer.perOperationRateLimiter.Set(operationName, rateLimiter)
	}
	credits := rateLimiter.Withdraw(maxWithdrawal)
	return credits
}

// CreditStore manages the relationship between service instance clients and
// backend storage credits. Each client must withdraw credits in order to submit
// trace data to the Jaeger agent. These credits are generated on behalf of the
// service, of which there can be many instances, and thus many clients.
type CreditStore interface {
	io.Closer
	// Withdraw deducts as many credits from the service on behalf of the client
	// without exceeding MaxClientBalance. The client is identified by a unique
	// clientID string. The credits are deducted from the specific operation on
	// the service if present, otherwise from the default credit pool.
	Withdraw(clientID string, serviceName string, operationName string) float64
	// Spend credits already allocated to this client on a given operation.
	Spend(clientID string, operationName string, credits float64) bool
}

type creditStore struct {
	sync.Mutex
	creditAccruers map[string]*creditAccruer
	clients        map[string]*client
	options        CreditAccruerOptions
	ttl            time.Duration
	timeNow        func() time.Time // For testing
	ticker         *time.Ticker
	done           chan bool
	waitGroup      sync.WaitGroup
}

// NewCreditStore creates a new creditStore and returns it to the caller.
// options should be the default values passed to new creditAccruers.
// ttl should be the TTL for all creditAccruers and clients.
func NewCreditStore(options CreditAccruerOptions, ttl time.Duration) CreditStore {
	s := &creditStore{
		creditAccruers: map[string]*creditAccruer{},
		clients:        map[string]*client{},
		options:        options,
		ttl:            ttl,
	}
	s.timeNow = time.Now
	s.ticker = time.NewTicker(10 * time.Second)
	s.done = make(chan bool)
	s.waitGroup.Add(1)
	go func() {
		defer s.waitGroup.Done()
		for {
			select {
			case <-s.ticker.C:
				s.purgeExpired()
			case <-s.done:
				return
			}
		}
	}()
	return s
}

func (s *creditStore) Withdraw(clientID string, serviceName string, operationName string) float64 {
	s.Lock()
	defer s.Unlock()
	c := s.findOrCreateClient(clientID)
	accruer := s.findOrCreateCreditAccruer(serviceName)
	balance := c.perOperationBalance[operationName]
	maxWithdrawal := s.options.ClientMaxBalance - balance
	credits := accruer.Withdraw(operationName, maxWithdrawal)
	now := s.timeNow()
	c.updateTime = now
	accruer.updateTime = now
	c.perOperationBalance[operationName] += credits
	return credits
}

func (s *creditStore) Spend(clientID string, operationName string, credits float64) bool {
	s.Lock()
	defer s.Unlock()
	c := s.findOrCreateClient(clientID)
	c.updateTime = s.timeNow()
	return c.Spend(operationName, credits)
}

func (s *creditStore) Close() error {
	s.done <- true
	close(s.done)

	s.waitGroup.Wait()

	s.Lock()
	s.ticker.Stop()
	s.Unlock()

	return nil
}

func (s *creditStore) findOrCreateCreditAccruer(serviceName string) *creditAccruer {
	accruer, ok := s.creditAccruers[serviceName]
	if !ok {
		accruer = newCreditAccruer(serviceName, s.options, s.timeNow)
		s.creditAccruers[serviceName] = accruer
	}
	return accruer
}

func (s *creditStore) findOrCreateClient(id string) *client {
	c, ok := s.clients[id]
	if !ok {
		c = newClient(id, s.timeNow())
		s.clients[id] = c
	}
	return c
}

func (s *creditStore) purgeExpired() {
	s.Lock()
	defer s.Unlock()
	for _, accruer := range s.creditAccruers {
		if s.timeNow().Sub(accruer.updateTime) >= s.ttl {
			delete(s.creditAccruers, accruer.serviceName)
		}
	}
	for _, client := range s.clients {
		if s.timeNow().Sub(client.updateTime) >= s.ttl {
			delete(s.clients, client.id)
		}
	}
}
