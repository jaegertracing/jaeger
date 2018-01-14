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
	"errors"
	"fmt"
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

func newTokenBucket(options AccountOptions, timeNow func() time.Time) *tokenBucket {
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
// services and clients. The clients must request credits from the service't
// account and they all share the same underlying tokenBucket. For example, if
// one client requests a withdrawal and receives all the service credits, the
// next client withdrawal request will be rejected until the credits refill.
type client struct {
	// perOperationBalance maintains per-operation balances of withdrawn credits
	perOperationBalance map[string]float64
	// updateTime is the last time client was updated
	updateTime time.Time
}

func newClient(currentTime time.Time) *client {
	return &client{
		updateTime:          currentTime,
		perOperationBalance: map[string]float64{},
	}
}

// Spend depletes the balance from the client for the given operation. The usage
// would be when a client submits spans for operation X, it is spending those
// credits it requested for operation X.
func (c *client) Spend(operationName string, credits float64) error {
	balance := c.perOperationBalance[operationName]
	var err error
	if credits > balance {
		err = errors.New(
			fmt.Sprintf("Overspending occurred: "+
				"balance %v, credits spent %v, operationName %v",
				balance, credits, operationName))
		balance = 0
	} else {
		balance -= credits
	}
	c.perOperationBalance[operationName] = balance
	return err
}

// AccountOptions provides values to be used with an account object.
type AccountOptions struct {
	// MaxOperations defines the maximum number of operation specific token
	// buckets an account may maintain.
	MaxOperations int
	// CreditsPerSecond defines the regeneration rate of the account's internal
	// token buckets.
	CreditsPerSecond float64
	// MaxBalance defines the maximum amount of credits in a token bucket before
	// it will no longer accrue credits/tokens (until credits are used).
	MaxBalance float64
}

// account represents a service-level credit account. Internally, it maintains
// a token bucket that replenishes its supply of credits as time goes on.
// Multiple clients can share the same service account, so the account credits
// are shared among them, regardless of the number of clients. The account will
// maintain a number of token buckets for specific operations, falling back on
// a default token bucket for other operations.
type account struct {
	options                 AccountOptions
	perOperationRateLimiter *overrideMap
	updateTime              time.Time
	timeNow                 func() time.Time // For testing
}

func newAccount(options AccountOptions, timeNow func() time.Time) *account {
	defaultRateLimiter := newTokenBucket(options, timeNow)
	perOperationRateLimiter := newOverrideMap(options.MaxOperations, defaultRateLimiter)
	return &account{
		options:                 options,
		updateTime:              timeNow(),
		timeNow:                 timeNow,
		perOperationRateLimiter: perOperationRateLimiter,
	}
}

// Withdraw deducts credits from the account for the given operationName with an
// upper limit of maxWithdrawal.
func (a *account) Withdraw(operationName string, maxWithdrawal float64) float64 {
	var rateLimiter *tokenBucket
	if a.perOperationRateLimiter.Has(operationName) ||
		a.perOperationRateLimiter.IsFull() {
		rateLimiter = a.perOperationRateLimiter.Get(operationName).(*tokenBucket)
	} else {
		rateLimiter = newTokenBucket(a.options, a.timeNow)
		a.perOperationRateLimiter.Set(operationName, rateLimiter)
	}
	credits := rateLimiter.Withdraw(maxWithdrawal)
	return credits
}

// ThrottlerOptions provides values to be used in a Throttler object.
type ThrottlerOptions struct {
	// DefaultAccountOptions defines the default AccountOptions to use for all
	// service accounts.
	DefaultAccountOptions AccountOptions
	// AccountOptionOverrides overrides DefaultAccountOptions for services with
	// service names present in the map.
	AccountOptionOverrides map[string]*AccountOptions

	// ClientMaxBalance defines the maximum balance a client may maintain before
	// further Withdraw calls return zero.
	ClientMaxBalance float64

	// TTL defines the time to await further updates before purging entry for
	// all accounts and clients.
	TTL time.Duration
	// PurgeInterval defines the amount of time to wait between purge calls.
	PurgeInterval time.Duration
}

// Throttler manages the relationship between service instance clients and
// backend storage credits. Each client must withdraw credits in order to submit
// trace data to the Jaeger agent. These credits are generated on behalf of the
// service, of which there can be many instances, and thus many clients.
type Throttler struct {
	sync.Mutex
	accounts  map[string]*account
	clients   map[string]*client
	options   ThrottlerOptions
	timeNow   func() time.Time // For testing
	ticker    *time.Ticker
	done      chan bool
	waitGroup sync.WaitGroup
}

// NewThrottler creates a new throttler and returns it to the caller.
// options should specify the specific throttler options to use.
func NewThrottler(options ThrottlerOptions) *Throttler {
	t := &Throttler{
		accounts: map[string]*account{},
		clients:  map[string]*client{},
		options:  options,
		timeNow:  time.Now,
	}
	t.ticker = time.NewTicker(t.options.PurgeInterval)
	t.done = make(chan bool)
	t.waitGroup.Add(1)
	go func() {
		defer t.waitGroup.Done()
		for {
			select {
			case <-t.ticker.C:
				t.purgeExpired()
			case <-t.done:
				return
			}
		}
	}()
	return t
}

// Withdraw deducts as many credits from the service on behalf of the client
// without exceeding MaxClientBalance. The client is identified by a unique
// clientID string. The credits are deducted from the specific operation on
// the service if present, otherwise from the default credit pool.
func (t *Throttler) Withdraw(serviceName string, clientID string, operationName string) float64 {
	t.Lock()
	defer t.Unlock()
	c := t.findOrCreateClient(clientID)
	a := t.findOrCreateAccount(serviceName)
	balance := c.perOperationBalance[operationName]
	maxWithdrawal := t.options.ClientMaxBalance - balance
	credits := a.Withdraw(operationName, maxWithdrawal)
	now := t.timeNow()
	c.updateTime = now
	a.updateTime = now
	c.perOperationBalance[operationName] += credits
	return credits
}

// Spend spends credits already allocated to this client on a given operation.
func (t *Throttler) Spend(serviceName string, clientID string, operationName string, credits float64) error {
	t.Lock()
	defer t.Unlock()
	c := t.findOrCreateClient(clientID)
	c.updateTime = t.timeNow()
	err := c.Spend(operationName, credits)
	if err != nil {
		err = errors.New(err.Error() + ", serviceName " + serviceName)
	}
	return err
}

// Close closes the throttler and stops all background goroutines.
func (t *Throttler) Close() error {
	t.done <- true
	close(t.done)

	t.waitGroup.Wait()

	t.Lock()
	t.ticker.Stop()
	t.Unlock()

	return nil
}

func (t *Throttler) findOrCreateAccount(serviceName string) *account {
	a, ok := t.accounts[serviceName]
	if !ok {
		accountOptions := t.options.DefaultAccountOptions
		if t.options.AccountOptionOverrides != nil {
			if o, ok := t.options.AccountOptionOverrides[serviceName]; ok && o != nil {
				accountOptions = *o
			}
		}
		a = newAccount(accountOptions, t.timeNow)
		t.accounts[serviceName] = a
	}
	return a
}

func (t *Throttler) findOrCreateClient(clientID string) *client {
	c, ok := t.clients[clientID]
	if !ok {
		c = newClient(t.timeNow())
		t.clients[clientID] = c
	}
	return c
}

func (t *Throttler) purgeExpired() {
	t.Lock()
	defer t.Unlock()
	for serviceName, a := range t.accounts {
		if t.timeNow().Sub(a.updateTime) >= t.options.TTL {
			delete(t.accounts, serviceName)
		}
	}
	for clientID, client := range t.clients {
		if t.timeNow().Sub(client.updateTime) >= t.options.TTL {
			delete(t.clients, clientID)
		}
	}
}
