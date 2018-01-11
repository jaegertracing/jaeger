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
)

func TestCreditStore(t *testing.T) {
	ts := time.Now()
	mockTime := func() time.Time { return ts }

	options := CreditAccruerOptions{
		MaxOperations:    1,
		CreditsPerSecond: 1,
		MaxBalance:       3,
		ClientMaxBalance: 2,
	}
	store := NewCreditStore(options, 10*time.Minute)
	store.timeNow = mockTime

	clientID := "test-client"
	serviceName := "test-service"
	operationNames := []string{"test-operation1", "test-operation2", "test-operation3"}
	credits := store.Withdraw(clientID, serviceName, operationNames[0])
	assert.Equal(t, 2.0, credits)

	// Wait a second without spending and try to get more tokens
	ts = ts.Add(time.Second)
	credits = store.Withdraw(clientID, serviceName, operationNames[0])
	assert.Equal(t, 0.0, credits)

	// Now spend all and try for the 2 tokens again
	success := store.Spend(clientID, operationNames[0], 2)
	assert.True(t, success)

	// Make sure double-spending does not work
	success = store.Spend(clientID, operationNames[0], 2)
	assert.False(t, success)

	credits = store.Withdraw(clientID, serviceName, operationNames[0])
	assert.Equal(t, 2.0, credits)
	success = store.Spend(clientID, operationNames[0], 2)
	assert.True(t, success)

	// Make sure new operations go to default balance/creditAccruer by withdrawing
	// from two new operations. If we can withdraw from both, we are not using default.
	credits = store.Withdraw(clientID, serviceName, operationNames[1])
	assert.Equal(t, 2.0, credits)
	credits = store.Withdraw(clientID, serviceName, operationNames[2])

	// Make sure default credits do not work for the earlier operation
	success = store.Spend(clientID, operationNames[0], 2.0)
	assert.False(t, success)

	// Make sure default credits work for the default operations
	success = store.Spend(clientID, operationNames[1], 1.0)
	assert.True(t, success)
	success = store.Spend(clientID, operationNames[2], 1.0)
	assert.True(t, success)

	ts = ts.Add(10 * time.Minute)
	store.PurgeExpired()
	assert.Equal(t, 0, len(store.clients))
	assert.Equal(t, 0, len(store.creditAccruers))
}
