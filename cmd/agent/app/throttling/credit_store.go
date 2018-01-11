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
	"math"
	"time"
)

type tokenBucket struct {
	creditsPerSecond float64
	balance          float64
	maxBalance       float64
	lastTick         time.Time
	timeNow          func() time.Time // For testing
}

func (t *tokenBucket) Drain(maxDrain float64) float64 {
	now := t.timeNow()
	interval := now.Sub(t.lastTick)
	t.lastTick = now
	diff := interval.Seconds() * t.creditsPerSecond
	t.balance = math.Min(t.balance+diff, t.maxBalance)
	result := math.Min(t.balance, maxDrain)
	t.balance -= result
	return result
}

type client struct {
	id                  string
	perOperationBalance map[string]float64
	defaultBalance      float64
	updateTime          time.Time
}

func (c *client) Spend(operationName string, credits float64) bool {
	balance, ok := c.perOperationBalance[operationName]
	if !ok {
		balance = c.defaultBalance
	}

	if credits > balance {
		return false
	}

	balance -= credits
	if ok {
		c.perOperationBalance[operationName] = balance
	} else {
		c.defaultBalance = balance
	}
	return true
}

type CreditAccruerOptions struct {
	MaxOperations    int
	CreditsPerSecond float64
	MaxBalance       float64
	ClientMaxBalance float64
}

type creditAccruer struct {
	options                 CreditAccruerOptions
	serviceName             string
	perOperationRateLimiter map[string]*tokenBucket
	defaultRateLimiter      *tokenBucket
	updateTime              time.Time
	timeNow                 func() time.Time // For testing
}

func (ca *creditAccruer) Withdraw(operationName string, balance float64) float64 {
	if balance >= ca.options.ClientMaxBalance {
		return 0
	}
	rateLimiter, ok := ca.perOperationRateLimiter[operationName]
	if !ok {
		if len(ca.perOperationRateLimiter) < ca.options.MaxOperations {
			rateLimiter = &tokenBucket{
				creditsPerSecond: ca.options.CreditsPerSecond,
				balance:          ca.options.MaxBalance,
				maxBalance:       ca.options.MaxBalance,
				timeNow:          ca.timeNow,
				lastTick:         ca.timeNow(),
			}
			ca.perOperationRateLimiter[operationName] = rateLimiter
		} else {
			rateLimiter = ca.defaultRateLimiter
		}
	}

	credits := rateLimiter.Drain(ca.options.ClientMaxBalance - balance)
	return credits
}

type creditStore struct {
	creditAccruers map[string]*creditAccruer
	clients        map[string]*client
	defaultOptions CreditAccruerOptions
	lifetime       time.Duration
	timeNow        func() time.Time // For testing
}

func NewCreditStore(defaultOptions CreditAccruerOptions, lifetime time.Duration) *creditStore {
	s := &creditStore{
		creditAccruers: map[string]*creditAccruer{},
		clients:        map[string]*client{},
		defaultOptions: defaultOptions,
		lifetime:       lifetime,
	}
	s.timeNow = time.Now
	return s
}

func (s *creditStore) Withdraw(clientID string, serviceName string, operationName string) float64 {
	c := s.findOrCreateClient(clientID)
	ca := s.findOrCreateCreditAccruer(serviceName)

	// To keep client consistent with creditAccruer, we need to see if creditAccruer
	// will create a new perOperationRateLimiter for this operation and if so
	// create a new perOperationBalance on the client.
	var balance float64
	if _, ok := ca.perOperationRateLimiter[operationName]; ok {
		balance = c.perOperationBalance[operationName]
	} else {
		if len(ca.perOperationRateLimiter) < ca.options.MaxOperations {
			c.perOperationBalance[operationName] = balance
		} else {
			balance = c.defaultBalance
		}
	}

	credits := ca.Withdraw(operationName, balance)
	now := s.timeNow()
	c.updateTime = now
	ca.updateTime = now
	if _, ok := ca.perOperationRateLimiter[operationName]; ok {
		c.perOperationBalance[operationName] += credits
	} else {
		c.defaultBalance += credits
	}
	return credits
}

func (s *creditStore) Spend(clientID string, operationName string, credits float64) bool {
	c := s.findOrCreateClient(clientID)
	c.updateTime = s.timeNow()
	return c.Spend(operationName, credits)
}

func (s *creditStore) findOrCreateCreditAccruer(serviceName string) *creditAccruer {
	ca, ok := s.creditAccruers[serviceName]
	if !ok {
		ca = &creditAccruer{
			options:                 s.defaultOptions,
			serviceName:             serviceName,
			updateTime:              s.timeNow(),
			timeNow:                 s.timeNow,
			perOperationRateLimiter: map[string]*tokenBucket{},
			defaultRateLimiter: &tokenBucket{
				creditsPerSecond: s.defaultOptions.CreditsPerSecond,
				balance:          s.defaultOptions.MaxBalance,
				maxBalance:       s.defaultOptions.MaxBalance,
				timeNow:          s.timeNow,
				lastTick:         s.timeNow(),
			},
		}
		s.creditAccruers[serviceName] = ca
	}
	return ca
}

func (s *creditStore) findOrCreateClient(id string) *client {
	c, ok := s.clients[id]
	if !ok {
		c = &client{
			id:                  id,
			updateTime:          s.timeNow(),
			perOperationBalance: map[string]float64{},
		}
		s.clients[id] = c
	}
	return c
}

func (s *creditStore) PurgeExpired() {
	for _, ca := range s.creditAccruers {
		if s.timeNow().Sub(ca.updateTime) >= s.lifetime {
			delete(s.creditAccruers, ca.serviceName)
		}
	}
	for _, client := range s.clients {
		if s.timeNow().Sub(client.updateTime) >= s.lifetime {
			delete(s.clients, client.id)
		}
	}
}
