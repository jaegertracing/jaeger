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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	thriftGen "github.com/jaegertracing/jaeger/thrift-gen/throttling"
)

type fakeThrottlingService struct {
	thriftGen.TChanThrottlingService
	throttlerConfig ThrottlerConfig
}

func (f *fakeThrottlingService) GetThrottlingConfigs(
	ctx thrift.Context,
	serviceNames []string,
) (*thriftGen.ThrottlingResponse, error) {
	response := thriftGen.NewThrottlingResponse()
	for _, serviceName := range serviceNames {
		if accountCfg, ok := f.throttlerConfig.AccountConfigOverrides[serviceName]; ok {
			serviceCfg := thriftGen.NewServiceThrottlingConfig()
			serviceCfg.ServiceName = serviceName
			serviceCfg.Config = thriftGen.NewThrottlingConfig()
			serviceCfg.Config.CreditsPerSecond = accountCfg.CreditsPerSecond
			serviceCfg.Config.MaxBalance = accountCfg.MaxBalance
			serviceCfg.Config.MaxOperations = int32(accountCfg.MaxOperations)
			response.ServiceConfigs = append(response.ServiceConfigs, serviceCfg)
		}
	}
	response.DefaultConfig = thriftGen.NewThrottlingConfig()
	response.DefaultConfig.CreditsPerSecond = f.throttlerConfig.DefaultAccountConfig.CreditsPerSecond
	response.DefaultConfig.MaxBalance = f.throttlerConfig.DefaultAccountConfig.MaxBalance
	response.DefaultConfig.MaxOperations = int32(f.throttlerConfig.DefaultAccountConfig.MaxOperations)
	return response, nil
}

func startServer(t *testing.T, serviceName string, throttlerConfig ThrottlerConfig) *tchannel.Channel {
	ch, err := tchannel.NewChannel(serviceName, nil)
	require.NoError(t, err)
	handler := &fakeThrottlingService{throttlerConfig: throttlerConfig}
	server := thrift.NewServer(ch)
	server.Register(thriftGen.NewTChanThrottlingServiceServer(handler))
	err = ch.ListenAndServe("localhost:0")
	require.NoError(t, err)
	require.Equal(t, tchannel.ChannelListening, ch.State())
	return ch
}

func startClient(t *testing.T, serviceName string, hostPort string) *tchannel.Channel {
	const (
		timeout = 100 * time.Millisecond
	)
	ch, err := tchannel.NewChannel(serviceName, nil)
	ch.Peers().Add(hostPort)
	require.NoError(t, err)
	require.Equal(t, tchannel.ChannelClient, ch.State())
	return ch
}

func newTestingThrottler(t *testing.T, cfg ThrottlerConfig) *Throttler {
	const (
		serviceName = "jaeger-throttling"
	)

	s := startServer(t, serviceName, cfg)
	ch := startClient(t, serviceName, s.PeerInfo().HostPort)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	throttler := NewThrottler(ch, serviceName, logger, cfg)
	require.NotNil(t, throttler)
	return throttler
}

func TestThrottler(t *testing.T) {
	ts := time.Now()
	var mu sync.Mutex
	mockTime := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return ts
	}

	cfg := ThrottlerConfig{
		DefaultAccountConfig: AccountConfig{
			MaxOperations:    1,
			CreditsPerSecond: 1,
			MaxBalance:       3,
		},
		AccountConfigOverrides: map[string]*AccountConfig{},
		ClientMaxBalance:       2,
		InactiveEntryLifetime:  10 * time.Second,
		PurgeInterval:          100 * time.Millisecond,
		ConfigRefreshInterval:  100 * time.Millisecond,
		ConfigRefreshJitter:    10 * time.Millisecond,
	}

	specialServiceName := "special-service"
	cfg.AccountConfigOverrides[specialServiceName] = &AccountConfig{
		MaxOperations:    1,
		CreditsPerSecond: 1,
		MaxBalance:       2,
	}

	throttler := newTestingThrottler(t, cfg)
	defer throttler.Close()
	throttler.timeNow = mockTime

	clientID := "test-client"
	serviceName := "test-service"
	operationNames := []string{"test-operation1", "test-operation2", "test-operation3"}
	credits := throttler.Withdraw(serviceName, clientID, operationNames[0])
	assert.Equal(t, 2.0, credits)

	// Wait a second without spending and try to get more tokens
	mu.Lock()
	ts = ts.Add(time.Second)
	mu.Unlock()
	credits = throttler.Withdraw(serviceName, clientID, operationNames[0])
	assert.Equal(t, 0.0, credits)

	// Now spend all and try for the 2 tokens again
	err := throttler.Spend(serviceName, clientID, operationNames[0], 2)
	require.NoError(t, err)

	// Make sure double-spending does not work
	err = throttler.Spend(serviceName, clientID, operationNames[0], 2)
	require.Error(t, err)

	credits = throttler.Withdraw(serviceName, clientID, operationNames[0])
	assert.Equal(t, 2.0, credits)
	err = throttler.Spend(serviceName, clientID, operationNames[0], 2)
	require.NoError(t, err)

	// Make sure new operations go to default balance/account by withdrawing
	// from two new operations. If we can withdraw from both, we are not using default.
	credits = throttler.Withdraw(serviceName, clientID, operationNames[1])
	assert.Equal(t, 2.0, credits)
	credits = throttler.Withdraw(serviceName, clientID, operationNames[2])

	// Make sure default credits do not work for the earlier operation
	err = throttler.Spend(serviceName, clientID, operationNames[0], 2.0)
	require.Error(t, err)

	// Make sure default credits work for the default operations
	err = throttler.Spend(serviceName, clientID, operationNames[1], 1.0)
	require.NoError(t, err)
	err = throttler.Spend(serviceName, clientID, operationNames[2], 1.0)
	require.NoError(t, err)

	// Test service-specific config override
	specialClientID := "special-client"
	credits = throttler.Withdraw(specialServiceName, specialClientID, operationNames[0])
	assert.Equal(t, 2.0, credits)
	err = throttler.Spend(specialClientID, specialClientID, operationNames[0], credits)
	require.NoError(t, err)

	// Try updating config for "special-service" and see max we can withdraw
	mu.Lock()
	ts = ts.Add(2 * time.Second)
	mu.Unlock()
	accountConfig := AccountConfig{
		MaxOperations:    1,
		CreditsPerSecond: 1,
		MaxBalance:       1, // Change from 2 -> 1
	}
	throttler.updateServiceConfig(specialServiceName, accountConfig)
	credits = throttler.Withdraw(specialServiceName, specialClientID, operationNames[0])
	assert.Equal(t, 1.0, credits)

	// Check that serviceNames works
	serviceNames := throttler.serviceNames()
	assert.Contains(t, serviceNames, serviceName)
	assert.Contains(t, serviceNames, specialServiceName)
	assert.Len(t, serviceNames, 2)

	// Test purge mechanism
	mu.Lock()
	ts = ts.Add(cfg.InactiveEntryLifetime)
	mu.Unlock()
	time.Sleep(cfg.PurgeInterval * 2)

	throttler.Lock()
	assert.Len(t, throttler.clients, 0)
	assert.Len(t, throttler.accounts, 0)
	throttler.Unlock()
}
