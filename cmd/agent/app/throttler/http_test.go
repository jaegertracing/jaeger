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
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHTTP(t *testing.T) {
	const (
		creditsPerSecond = 1.0
		serviceName      = "test-service"
	)
	cfg := ThrottlerConfig{
		DefaultAccountConfig: AccountConfig{
			MaxOperations:    3,
			CreditsPerSecond: creditsPerSecond,
			MaxBalance:       3,
		},
		AccountConfigOverrides: map[string]*AccountConfig{},
		ClientMaxBalance:       2,
		InactiveEntryLifetime:  10 * time.Second,
		PurgeInterval:          100 * time.Millisecond,
		ConfigRefreshInterval:  100 * time.Millisecond,
	}
	throttler := newTestingThrottler(t, cfg)
	defer throttler.Close()
	service := NewHTTPService(throttler)
	mux := http.NewServeMux()
	mux.Handle("/credits", http.HandlerFunc(service.Credits))
	mux.Handle("/accounts", http.HandlerFunc(service.Accounts))
	mux.Handle("/clients", http.HandlerFunc(service.Clients))
	startTime := time.Now()
	server := httptest.NewServer(mux)
	defer server.Close()

	client := server.Client()

	testCases := []struct {
		query   url.Values
		status  int
		message string
		payload creditResponse
	}{
		{
			status:  http.StatusBadRequest,
			message: "'service' parameter must be provided once\n",
		},
		{
			query:   url.Values{"service": []string{serviceName}},
			status:  http.StatusBadRequest,
			message: "'uuid' parameter must be provided once\n",
		},
		{
			query: url.Values{
				"service": {serviceName},
				"uuid":    {"5"},
			},
			status:  http.StatusBadRequest,
			message: "'operations' parameter must be provided at least once\n",
		},
		{
			query: url.Values{
				"service":    {serviceName},
				"uuid":       {"5"},
				"operations": {"a", "b"},
			},
			status: http.StatusOK,
			payload: creditResponse{
				Balances: []OperationBalance{
					{Operation: "a", Balance: 2},
					{Operation: "b", Balance: 2},
				},
			},
		},
		{
			query: url.Values{
				"service":    {serviceName},
				"uuid":       {"5"},
				"operations": {"a", "b"},
			},
			status: http.StatusOK,
			payload: creditResponse{
				Balances: []OperationBalance{
					{Operation: "a", Balance: 0},
					{Operation: "b", Balance: 0},
				},
			},
		},
		{
			query: url.Values{
				"service":    {serviceName},
				"uuid":       {"6"},
				"operations": {"a"},
			},
			status: http.StatusOK,
			payload: creditResponse{
				Balances: []OperationBalance{
					{Operation: "a", Balance: 1},
				},
			},
		},
	}

	for _, testCase := range testCases {
		serverURL := server.URL + "/credits?" + testCase.query.Encode()
		res, err := client.Get(serverURL)
		assert.NoError(t, err)
		assert.Equal(t, testCase.status, res.StatusCode)
		message, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		assert.NoError(t, err)
		if res.StatusCode == http.StatusOK {
			var clientBalances creditResponse
			err := json.Unmarshal(message, &clientBalances)
			assert.NoError(t, err)
			// Allow response to deviate from expected credit values. The delta
			// is set to the amount of time passed multiplied by the token
			// bucket refill rate. The reason for this is that the expected
			// payload assumes no time has transpired between requests, which is
			// not realistic.
			delta := time.Since(startTime).Seconds() * creditsPerSecond
			assert.InDeltaMapValues(
				t,
				perOperationBalanceListToMap(clientBalances.Balances),
				perOperationBalanceListToMap(testCase.payload.Balances),
				delta,
			)
		} else {
			assert.Equal(t, testCase.message, string(message))
		}
	}

	res, err := client.Get(server.URL + "/accounts")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	message, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	res.Body.Close()
	accounts := map[string]AccountSnapshot{}
	err = json.Unmarshal(message, &accounts)
	assert.NoError(t, err)
	assert.Len(t, accounts, 1)
	assert.NotEqual(t, AccountSnapshot{}, accounts[serviceName])

	res, err = client.Get(server.URL + "/clients")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	message, err = ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	res.Body.Close()
	serviceMap := map[string]ClientSnapshot{}
	err = json.Unmarshal(message, &serviceMap)
	assert.NoError(t, err)
	assert.Len(t, serviceMap[serviceName].ClientBalancesByUUID, 2)
	assert.Len(t, serviceMap[serviceName].ClientBalancesByUUID["5"], 2)
	assert.Len(t, serviceMap[serviceName].ClientBalancesByUUID["6"], 1)
}

func perOperationBalanceListToMap(balances []OperationBalance) map[string]float64 {
	balanceMap := map[string]float64{}
	for _, opBalance := range balances {
		balanceMap[opBalance.Operation] = opBalance.Balance
	}
	return balanceMap
}

func findOperation(balances []OperationBalance, operation string) *OperationBalance {
	for _, opBalance := range balances {
		if opBalance.Operation == operation {
			return &opBalance
		}
	}
	return nil
}

func TestJSONSerializationError(t *testing.T) {
	var fakeError = errors.New("fake")
	mockMarshaller := func(x interface{}) ([]byte, error) {
		return nil, fakeError
	}
	recorder := httptest.NewRecorder()
	makeJSONResponse(struct{}{}, recorder, mockMarshaller)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Failed to encode result as JSON")
}
