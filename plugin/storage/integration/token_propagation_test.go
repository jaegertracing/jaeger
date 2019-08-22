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

// +build token_propagation

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bearerToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbiIsIm5hbWUiOiJKb2huIERvZSIsImlhdCI"
const bearerHeader = "Bearer " + bearerToken
const queryUrl = "http://127.0.0.1:16686/api/services"

type esTokenPropagationTestHandler struct {
	test *testing.T
}

func (h *esTokenPropagationTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Return the elasticsearch version
	if r.URL.Path == "/" {
		ret := new(elastic.PingResult)
		ret.Version.Number = "7.3.0"
		json_ret, _ := json.Marshal(ret)
		w.Header().Add("Content-Type", "application/json; charset=UTF-8")
		w.Write(json_ret)
		return
	}

	authValue := r.Header.Get("Authorization")
	if authValue != "" {
		headerValue := strings.Split(authValue, " ")
		token := ""
		if len(headerValue) == 2 {
			if headerValue[0] == "Bearer" {
				token = headerValue[1]
			}
		} else if len(headerValue) == 1 {
			token = authValue
		}
		assert.Equal(h.test, bearerToken, token)
		if token == bearerToken {
			// Return empty results, we don't care about the result here.
			// we just need to make sure the token was propagated to the storage and the query-service returns 200
			ret := new(elastic.SearchResult)
			json_ret, _ := json.Marshal(ret)
			w.Header().Add("Content-Type", "application/json; charset=UTF-8")
			w.Write(json_ret)
			return
		}
	}
	// No token, return error!
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

func createElasticSearchMock(srv *http.Server, t *testing.T) {
	srv.Handler = &esTokenPropagationTestHandler{
		test: t,
	}
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		assert.Error(t, err)
	}
}

func TestBearTokenPropagation(t *testing.T) {
	testCases := []struct {
		name        string
		headerValue string
		headerName  string
	}{
		{name: "Bearer token", headerName: "Authorization", headerValue: bearerHeader},
		{name: "Raw Bearer token", headerName: "Authorization", headerValue: bearerToken},
		{name: "X-Forwarded-Access-Token", headerName: "X-Forwarded-Access-Token", headerValue: bearerHeader},
	}

	// Run elastic search mocked server in background..
	// is not a full server, just mocked the necessary stuff for this test.
	srv := &http.Server{Addr: ":9200"}
	defer srv.Shutdown(context.Background())

	go createElasticSearchMock(srv, t)
	// Wait for http server to start
	time.Sleep(100 * time.Millisecond)

	// Path relative to plugin/storage/integration/token_propagation_test.go
	cmd := exec.Command("../../../cmd/query/query-linux", "--es.server-urls=http://127.0.0.1:9200", "--es.tls=false", "--query.bearer-token-propagation=true")
	cmd.Env = []string{"SPAN_STORAGE_TYPE=elasticsearch"}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	assert.NoError(t, err)

	// Wait for query service to start
	time.Sleep(100 * time.Millisecond)

	// Test cases.
	for _, testCase := range testCases {
		// Ask for services query, this should return 200
		req, err := http.NewRequest("GET", queryUrl, nil)
		require.NoError(t, err)
		req.Header.Add(testCase.headerName, testCase.headerValue)
		client := &http.Client{}
		resp, err := client.Do(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		if err == nil && resp != nil {
			assert.Equal(t, resp.StatusCode, http.StatusOK)
		}
	}

	cmd.Process.Kill()
}
