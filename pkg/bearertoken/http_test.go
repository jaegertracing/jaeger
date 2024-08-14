// Copyright (c) 2019 The Jaeger Authors.
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

package bearertoken

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func Test_PropagationHandler(t *testing.T) {
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}

	logger := zap.NewNop()
	const bearerToken = "blah"

	validTokenHandler := func(stop *sync.WaitGroup) http.HandlerFunc {
		return func(_ http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			token, ok := GetBearerToken(ctx)
			assert.Equal(t, bearerToken, token)
			assert.True(t, ok)
			stop.Done()
		}
	}

	emptyHandler := func(stop *sync.WaitGroup) http.HandlerFunc {
		return func(_ http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			token, _ := GetBearerToken(ctx)
			assert.Empty(t, token, bearerToken)
			stop.Done()
		}
	}

	testCases := []struct {
		name        string
		sendHeader  bool
		headerValue string
		headerName  string
		handler     func(stop *sync.WaitGroup) http.HandlerFunc
	}{
		{name: "Bearer token", sendHeader: true, headerName: "Authorization", headerValue: "Bearer " + bearerToken, handler: validTokenHandler},
		{name: "Raw bearer token", sendHeader: true, headerName: "Authorization", headerValue: bearerToken, handler: validTokenHandler},
		{name: "No headerValue", sendHeader: false, headerName: "Authorization", handler: emptyHandler},
		{name: "Basic Auth", sendHeader: true, headerName: "Authorization", headerValue: "Basic " + bearerToken, handler: emptyHandler},
		{name: "X-Forwarded-Access-Token", headerName: "X-Forwarded-Access-Token", sendHeader: true, headerValue: "Bearer " + bearerToken, handler: validTokenHandler},
		{name: "Invalid header", headerName: "X-Forwarded-Access-Token", sendHeader: true, headerValue: "Bearer " + bearerToken + " another stuff", handler: emptyHandler},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stop := sync.WaitGroup{}
			stop.Add(1)
			r := PropagationHandler(logger, testCase.handler(&stop))
			server := httptest.NewServer(r)
			defer server.Close()
			req, err := http.NewRequest(http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			if testCase.sendHeader {
				req.Header.Add(testCase.headerName, testCase.headerValue)
			}
			_, err = httpClient.Do(req)
			require.NoError(t, err)
			stop.Wait()
		})
	}
}
