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

package throttling

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	thriftGen "github.com/jaegertracing/jaeger/thrift-gen/throttling"
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

func newTokenBucket(cfg AccountConfig, timeNow func() time.Time) *tokenBucket {
	return &tokenBucket{
		creditsPerSecond: cfg.CreditsPerSecond,
		balance:          cfg.MaxBalance,
		maxBalance:       cfg.MaxBalance,
		timeNow:          timeNow,
		lastTick:         timeNow(),
	}
}

// updateBalance is a helper function to update the token bucket balance.
func (t *tokenBucket) updateBalance() {
	now := t.timeNow()
	interval := now.Sub(t.lastTick)
	t.lastTick = now
	diff := interval.Seconds() * t.creditsPerSecond
	t.balance = math.Min(t.balance+diff, t.maxBalance)
}

// Withdraw deducts as much of the total balance possible without exceeding
// maxWithdrawal.
func (t *tokenBucket) Withdraw(maxWithdrawal float64) float64 {
	t.updateBalance()
	result := math.Min(t.balance, maxWithdrawal)
	t.balance -= result
	return result
}

// Balance gets the current balance for an account. This method should only be
// used for administrative or testing purposes as the value returned is volatile
// and cannot be relied upon.
func (t *tokenBucket) Balance() float64 {
	t.updateBalance()
	return t.balance
}

// UpdateConfig updates the token bucket configuration values with a new
// AccountConfig. After calculating the current balance against the old
// values (i.e. t.creditsPerSecond), the token bucket limits the balance to
// the new maximum (cfg.MaxBalance).
func (t *tokenBucket) UpdateConfig(cfg AccountConfig) {
	t.updateBalance()
	t.maxBalance = cfg.MaxBalance
	t.balance = math.Min(t.balance, t.maxBalance)
	t.creditsPerSecond = cfg.CreditsPerSecond
}

// client represents a Jaeger-instrumented caller. Every Jaeger client
// corresponds to an instrumented service, but it may be one of many instances
// of that service. Therefore, there is a one-to-many relationship between
// services and clients. The clients must request credits from the service's
// account and they all share the same underlying token bucket. For example, if
// one client requests a withdrawal and receives all the service credits, the
// next client withdrawal request will be rejected until the credits refill.
type client struct {
	// perOperationBalance maintains per-operation balances of withdrawn credits
	perOperationBalance map[string]float64
	// updateTime is the last time client was updated
	updateTime time.Time
	// serviceName is the service this client belongs to
	serviceName string
}

func newClient(serviceName string, currentTime time.Time) *client {
	return &client{
		updateTime:          currentTime,
		perOperationBalance: map[string]float64{},
		serviceName:         serviceName,
	}
}

// Spend depletes the balance from the client for the given operation. The usage
// would be when a client submits spans for operation X, it is spending those
// credits it requested for operation X.
func (c *client) Spend(operationName string, credits float64) error {
	balance := c.perOperationBalance[operationName]
	var err error
	if credits > balance {
		err = fmt.Errorf("Overspending occurred: "+
			"balance %v, credits spent %v, operationName %v",
			balance, credits, operationName)
		balance = 0
	} else {
		balance -= credits
	}
	c.perOperationBalance[operationName] = balance
	return err
}

// account represents a service-level credit account. Internally, it maintains
// a token bucket that replenishes its supply of credits as time goes on.
// Multiple clients can share the same service account, so the account credits
// are shared among them, regardless of the number of clients. The account will
// maintain a number of token buckets for specific operations, falling back on
// a default token bucket for other operations.
type account struct {
	cfg                     AccountConfig
	perOperationTokenBucket overrideMap
	updateTime              time.Time
	timeNow                 func() time.Time // For testing
}

func newAccount(cfg AccountConfig, timeNow func() time.Time) *account {
	defaultTokenBucket := newTokenBucket(cfg, timeNow)
	perOperationTokenBucket := *newOverrideMap(cfg.MaxOperations, defaultTokenBucket)
	return &account{
		cfg:                     cfg,
		updateTime:              timeNow(),
		timeNow:                 timeNow,
		perOperationTokenBucket: perOperationTokenBucket,
	}
}

// Withdraw deducts credits from the account for the given operationName with an
// upper limit of maxWithdrawal.
func (a *account) Withdraw(operationName string, maxWithdrawal float64) float64 {
	var bucket *tokenBucket
	if a.perOperationTokenBucket.Has(operationName) ||
		a.perOperationTokenBucket.IsFull() {
		bucket = a.perOperationTokenBucket.Get(operationName).(*tokenBucket)
	} else {
		bucket = newTokenBucket(a.cfg, a.timeNow)
		a.perOperationTokenBucket.Set(operationName, bucket)
	}
	return bucket.Withdraw(maxWithdrawal)
}

// Balances returns a snapshot of the current balances in this account.
func (a *account) Balances() AccountSnapshot {
	balance := AccountSnapshot{
		DefaultBalance: a.perOperationTokenBucket.Default().(*tokenBucket).Balance(),
	}
	for _, operationName := range a.perOperationTokenBucket.Keys() {
		balance.Balances = append(balance.Balances, OperationBalance{
			Operation: operationName,
			Balance:   a.perOperationTokenBucket.Get(operationName).(*tokenBucket).Balance(),
		})
	}
	return balance
}

// UpdateConfig replaces the account's current AccountConfig with a new one.
func (a *account) UpdateConfig(cfg AccountConfig) {
	if a.cfg == cfg {
		return
	}
	a.perOperationTokenBucket.UpdateMaxOverrides(cfg.MaxOperations)
	for _, bucket := range a.perOperationTokenBucket.overrides {
		bucket.(*tokenBucket).UpdateConfig(cfg)
	}
	defaultBucket := a.perOperationTokenBucket.Default().(*tokenBucket)
	defaultBucket.UpdateConfig(cfg)
	a.cfg = cfg
}

// Throttler manages the relationship between service instance clients and
// backend storage credits. Each client must withdraw credits in order to submit
// trace data to the Jaeger agent. These credits are generated on behalf of the
// service, of which there can be many instances, and thus many clients.
type Throttler struct {
	sync.Mutex
	accounts            map[string]*account
	clients             map[string]*client
	cfg                 ThrottlerConfig
	logger              *zap.Logger
	timeNow             func() time.Time // For testing
	done                chan struct{}
	waitGroup           sync.WaitGroup
	configServiceClient thriftGen.TChanThrottlingService
}

// See oss/collectors/sampling/processor.go for original implementation.
func randomJitter(maxJitter time.Duration) time.Duration {
	return maxJitter/2 + time.Duration(rand.Int63n(int64(maxJitter)/2))
}

// NewThrottler creates a new throttler and returns it to the caller.
// cfg should specify the specific throttler cfg to use.
func NewThrottler(ch *tchannel.Channel, svc string, logger *zap.Logger, cfg ThrottlerConfig) *Throttler {
	const (
		defaultConfigRefreshInterval = 10 * time.Minute
		defaultMaxJitter             = 1 * time.Minute
	)

	t := &Throttler{
		accounts: map[string]*account{},
		clients:  map[string]*client{},
		cfg:      cfg,
		logger:   logger,
		timeNow:  time.Now,
		done:     make(chan struct{}),
	}
	t.spawnGoroutine(t.purgeExpired, t.cfg.PurgeInterval)

	configRefreshInterval := t.cfg.ConfigRefreshInterval
	if configRefreshInterval == 0 {
		configRefreshInterval = defaultConfigRefreshInterval
	}
	maxJitter := t.cfg.ConfigRefreshJitter
	if maxJitter == 0 {
		maxJitter = defaultMaxJitter
	}
	jitterAmount := randomJitter(maxJitter)
	thriftClient := thrift.NewClient(ch, svc, &thrift.ClientOptions{})
	t.configServiceClient = thriftGen.NewTChanThrottlingServiceClient(thriftClient)
	// Have the refresh function run once on throttler initialization, followed
	// by periodic updates. Both the initial run and the subsequent updates are
	// delayed by jitterAmount to avoid collector contention if multiple agents
	// start at once.
	time.AfterFunc(jitterAmount, t.refreshConfigs)
	configRefreshInterval += jitterAmount
	t.spawnGoroutine(t.refreshConfigs, configRefreshInterval)
	return t
}

func (t *Throttler) spawnGoroutine(f func(), interval time.Duration) {
	t.waitGroup.Add(1)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer t.waitGroup.Done()
		for {
			select {
			case <-ticker.C:
				f()
			case <-t.done:
				return
			}
		}
	}()
}

// Withdraw deducts as many credits from the service on behalf of the client
// without exceeding MaxClientBalance. The client is identified by a unique
// clientUUID string. The credits are deducted from the specific operation on
// the service if present, otherwise from the default credit pool.
func (t *Throttler) Withdraw(serviceName string, clientUUID string, operationName string) float64 {
	t.Lock()
	defer t.Unlock()
	c := t.findOrCreateClient(clientUUID, serviceName)
	a := t.findOrCreateAccount(serviceName)
	balance := c.perOperationBalance[operationName]
	maxWithdrawal := math.Max(0, t.cfg.ClientMaxBalance-balance)
	credits := a.Withdraw(operationName, maxWithdrawal)
	now := t.timeNow()
	c.updateTime = now
	a.updateTime = now
	c.perOperationBalance[operationName] += credits
	return credits
}

// accountSnapshot returns a map of balance snapshots, one for each account. The
// service name serves as the key to the service's account balance snapshot.
// These balance snapshots are useful for administrative purposes and testing,
// but are not meant for programmatic use in clients.
func (t *Throttler) accountSnapshot() map[string]AccountSnapshot {
	balances := map[string]AccountSnapshot{}
	t.Lock()
	defer t.Unlock()
	for serviceName, account := range t.accounts {
		balances[serviceName] = account.Balances()
	}
	return balances
}

func perOperationBalanceMapToList(m map[string]float64) []OperationBalance {
	result := make([]OperationBalance, 0, len(m))
	for op, balance := range m {
		result = append(result, OperationBalance{
			Operation: op,
			Balance:   balance,
		})
	}
	return result
}

// clientSnapshot returns a map of client balance snapshots. The map uses the
// service name as a key. The value contains a map of client IDs to client
// balances. N.B. These balance snapshots are useful for administrative purposes
// and testing, but are not meant for programmatic use in clients.
func (t *Throttler) clientSnapshot() map[string]ClientSnapshot {
	balances := map[string]ClientSnapshot{}
	t.Lock()
	defer t.Unlock()
	for clientUUID, client := range t.clients {
		if s, ok := balances[client.serviceName]; ok {
			s.ClientBalancesByUUID[clientUUID] = perOperationBalanceMapToList(client.perOperationBalance)
		} else {
			balances[client.serviceName] = ClientSnapshot{
				ClientBalancesByUUID: map[string][]OperationBalance{
					clientUUID: perOperationBalanceMapToList(client.perOperationBalance),
				},
			}
		}
	}
	return balances
}

// Spend spends credits already allocated to this client on a given operation.
func (t *Throttler) Spend(serviceName string, clientUUID string, operationName string, credits float64) error {
	t.Lock()
	defer t.Unlock()
	c := t.findOrCreateClient(clientUUID, serviceName)
	c.updateTime = t.timeNow()
	err := c.Spend(operationName, credits)
	if err != nil {
		err = fmt.Errorf("%s, serviceName %s", err.Error(), serviceName)
	}
	return err
}

// serviceNames returns a list of unique service names that are currently being
// monitored.
// N.B. Caller must be holder the throttler lock.
func (t *Throttler) serviceNames() []string {
	names := make([]string, 0, len(t.accounts))
	for serviceName := range t.accounts {
		names = append(names, serviceName)
	}
	return names
}

// updateServiceConfig replaces a service's current AccountConfig with a new
// one.
// N.B. Caller must be holding the throttler lock.
func (t *Throttler) updateServiceConfig(serviceName string, cfg AccountConfig) {
	if account, ok := t.accounts[serviceName]; ok {
		account.UpdateConfig(cfg)
	}
}

// updateDefaultConfig replaces the current default config and updates any
// service that is not listed in AccountConfigOverrides to use the new default
// config.
// N.B. Caller must be holding the throttler lock.
func (t *Throttler) updateDefaultConfig(cfg AccountConfig) {
	t.cfg.DefaultAccountConfig = cfg
	for _, serviceName := range t.serviceNames() {
		if _, ok := t.cfg.AccountConfigOverrides[serviceName]; !ok {
			account := t.accounts[serviceName]
			account.UpdateConfig(t.cfg.DefaultAccountConfig)
		}
	}
}

func (t *Throttler) refreshConfigs() {
	const (
		timeoutSeconds = 1
	)
	ctx, _ := thrift.NewContext(time.Second * timeoutSeconds)
	t.Lock()
	serviceNames := t.serviceNames()
	t.Unlock()
	response, err := t.configServiceClient.GetThrottlingConfigs(ctx, serviceNames)
	if response == nil || err != nil {
		t.logger.Error("GetThrottlingConfigs failed", zap.Any("response", response), zap.Error(err))
		return
	}
	if response.DefaultConfig != nil {
		newDefaultConfig := throttlingConfigToAccountConfig(*response.DefaultConfig)
		t.Lock()
		if *newDefaultConfig != t.cfg.DefaultAccountConfig {
			t.updateDefaultConfig(*newDefaultConfig)
		}
		t.Unlock()
	}

	newAccountOverrides := map[string]*AccountConfig{}
	for _, serviceConfig := range response.ServiceConfigs {
		if serviceConfig != nil {
			newAccountOverrides[serviceConfig.ServiceName] =
				throttlingConfigToAccountConfig(*serviceConfig.Config)
		}
	}

	t.mergeServiceConfigs(serviceNames, newAccountOverrides)
}

func (t *Throttler) mergeServiceConfigs(
	serviceNames []string, newAccountOverrides map[string]*AccountConfig) {
	t.Lock()
	defer t.Unlock()

	// The throttler must update t.cfg.AccountConfigOverrides, dealing with the
	// four cases depicted here:
	//
	// |-------|-------|
	// |  old  |  new  |
	// |-------|-------|
	// |  yes  |  yes  | => Update map config (case #1).
	// |-------|-------|
	// |  yes  |  no   | => Delete service name from map (case #2).
	// |-------|-------|
	// |  no   |  yes  | => Add override to map (case #3).
	// |-------|-------|
	// |  no   |  no   | => Don't do anything (no implementation).
	// |-------|-------|

	for _, serviceName := range serviceNames {
		_, presentInOldMap := t.cfg.AccountConfigOverrides[serviceName]
		newAccountConfig, presentInNewMap := newAccountOverrides[serviceName]

		if presentInNewMap {
			// Cases #1 and #3
			t.cfg.AccountConfigOverrides[serviceName] = newAccountConfig
			t.updateServiceConfig(serviceName, *newAccountConfig)
		} else if presentInOldMap {
			// Case #2
			delete(t.cfg.AccountConfigOverrides, serviceName)
			t.updateServiceConfig(serviceName, t.cfg.DefaultAccountConfig)
		}
	}
}

func throttlingConfigToAccountConfig(cfg thriftGen.ThrottlingConfig) *AccountConfig {
	return &AccountConfig{
		MaxOperations:    int(cfg.MaxOperations),
		CreditsPerSecond: cfg.CreditsPerSecond,
		MaxBalance:       cfg.MaxBalance,
	}
}

// Close closes the throttler and stops all background goroutines.
func (t *Throttler) Close() error {
	close(t.done)
	t.waitGroup.Wait()
	return nil
}

func (t *Throttler) findOrCreateAccount(serviceName string) *account {
	a, ok := t.accounts[serviceName]
	if !ok {
		accountConfig := t.cfg.DefaultAccountConfig
		if t.cfg.AccountConfigOverrides != nil {
			if o, ok := t.cfg.AccountConfigOverrides[serviceName]; ok && o != nil {
				accountConfig = *o
			}
		}
		a = newAccount(accountConfig, t.timeNow)
		t.accounts[serviceName] = a
	}
	return a
}

func (t *Throttler) findOrCreateClient(clientUUID string, serviceName string) *client {
	c, ok := t.clients[clientUUID]
	if !ok {
		c = newClient(serviceName, t.timeNow())
		t.clients[clientUUID] = c
	}
	return c
}

func (t *Throttler) purgeExpired() {
	t.Lock()
	defer t.Unlock()
	ttlDuration := t.cfg.InactiveEntryLifetime
	for serviceName, a := range t.accounts {
		if t.timeNow().Sub(a.updateTime) >= ttlDuration {
			delete(t.accounts, serviceName)
		}
	}
	for clientUUID, client := range t.clients {
		if t.timeNow().Sub(client.updateTime) >= ttlDuration {
			delete(t.clients, clientUUID)
		}
	}
}
