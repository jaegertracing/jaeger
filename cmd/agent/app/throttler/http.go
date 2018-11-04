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
	"net/http"
)

// OperationBalance is a pairing of operation name and its respective balance.
type OperationBalance struct {
	Operation string  `json:"operation"`
	Balance   float64 `json:"balance"`
}

type creditResponse struct {
	Balances []OperationBalance `json:"balances"`
}

// AccountSnapshot represents the number of credits per operation remaining for
// a given service.
type AccountSnapshot struct {
	Balances       []OperationBalance
	DefaultBalance float64
}

// ClientSnapshot maps each service client to its per-operation balance list
// using client UUIDs as keys.
type ClientSnapshot struct {
	ClientBalancesByUUID map[string][]OperationBalance `json:"clientBalancesByUUID"`
}

// HTTPService handles HTTP requests to withdraw from and view accounts of
// a wrapped throttler.
type HTTPService interface {
	// Credits exposes an HTTP route to withdraw credits from the throttler.
	Credits(w http.ResponseWriter, r *http.Request)
	// Accounts exposes an HTTP route to query the account balances.
	Accounts(w http.ResponseWriter, r *http.Request)
	// Clients exposes an HTTP route to query the client balances.
	Clients(w http.ResponseWriter, r *http.Request)
}

type httpService struct {
	throttler *Throttler
}

func (h *httpService) Credits(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	services := query["service"]
	clientUUIDs := query["uuid"]
	operations := query["operations"]
	if len(services) != 1 {
		http.Error(w, "'service' parameter must be provided once", http.StatusBadRequest)
		return
	}
	if len(clientUUIDs) != 1 {
		http.Error(w, "'uuid' parameter must be provided once", http.StatusBadRequest)
		return
	}
	if len(operations) == 0 {
		http.Error(w, "'operations' parameter must be provided at least once", http.StatusBadRequest)
		return
	}

	serviceName := services[0]
	clientUUID := clientUUIDs[0]
	var resp creditResponse
	for _, operation := range operations {
		resp.Balances = append(resp.Balances, OperationBalance{
			Operation: operation,
			Balance:   h.throttler.Withdraw(serviceName, clientUUID, operation),
		})
	}
	h.makeJSONResponse(resp, w)
}

func (h *httpService) makeJSONResponse(
	data interface{},
	w http.ResponseWriter,
) {
	makeJSONResponse(data, w, json.Marshal)
}

func makeJSONResponse(data interface{}, w http.ResponseWriter, marshaller func(interface{}) ([]byte, error)) {
	w.Header().Add("Content-Type", "application/json")
	jsonData, err := marshaller(data)
	if err != nil {
		http.Error(
			w,
			"Failed to encode result as JSON",
			http.StatusInternalServerError,
		)
		return
	}
	w.Write(jsonData)
}

func (h *httpService) Accounts(w http.ResponseWriter, r *http.Request) {
	h.makeJSONResponse(h.throttler.accountSnapshot(), w)
}

func (h *httpService) Clients(w http.ResponseWriter, r *http.Request) {
	h.makeJSONResponse(h.throttler.clientSnapshot(), w)
}

// NewHTTPService constructs a new HTTPService that accepts HTTP requests to
// withdraw credits on behalf of clients from accounts or query account
// balances.
func NewHTTPService(throttler *Throttler) HTTPService {
	return &httpService{throttler: throttler}
}
