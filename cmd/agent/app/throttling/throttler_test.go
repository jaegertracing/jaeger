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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThrottler(t *testing.T) {
	ts := time.Now()
	mockTime := func() time.Time { return ts }

	options := ThrottlerOptions{
		DefaultAccountOptions: AccountOptions{
			MaxOperations:    1,
			CreditsPerSecond: 1,
			MaxBalance:       3,
		},
		AccountOptionOverrides: map[string]*AccountOptions{},
		ClientMaxBalance:       2,
		TTL:                    10 * time.Minute,
		PurgeInterval:          200 * time.Millisecond,
	}

	specialServiceName := "special-service"
	options.AccountOptionOverrides[specialServiceName] = &AccountOptions{
		MaxOperations:    1,
		CreditsPerSecond: 1,
		MaxBalance:       2,
	}

	throttler := NewThrottler(options)
	throttler.timeNow = mockTime

	clientID := "test-client"
	serviceName := "test-service"
	operationNames := []string{"test-operation1", "test-operation2", "test-operation3"}
	credits := throttler.Withdraw(serviceName, clientID, operationNames[0])
	assert.Equal(t, 2.0, credits)

	// Wait a second without spending and try to get more tokens
	ts = ts.Add(time.Second)
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

	// Test service-specific options override
	specialClientID := "special-client"
	credits = throttler.Withdraw(specialServiceName, specialClientID, operationNames[0])
	assert.Equal(t, 2.0, credits)

	// Test purge mechanism
	ts = ts.Add(10 * time.Minute)
	time.Sleep(options.PurgeInterval)
	require.NoError(t, throttler.Close())
	assert.Equal(t, 0, len(throttler.clients))
	assert.Equal(t, 0, len(throttler.accounts))
}
