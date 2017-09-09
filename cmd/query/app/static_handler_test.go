// Copyright (c) 2017 Uber Technologies, Inc.
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

package app

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticAssetsHandler(t *testing.T) {
	r := mux.NewRouter()
	handler := NewStaticAssetsHandler("fixture")
	handler.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := httpClient.Get(server.URL)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestDefaultStaticAssetsRoot(t *testing.T) {
	handler := NewStaticAssetsHandler("")
	assert.Equal(t, "jaeger-ui-build/build/", handler.staticAssetsRoot)
}

func TestRegisterRoutesHandler(t *testing.T) {
	r := mux.NewRouter()
	handler := NewStaticAssetsHandler("fixture/")
	handler.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()
	expectedRespString := "Test Favicon\n"

	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := httpClient.Get(fmt.Sprintf("%s/favicon.ico", server.URL))
	require.NoError(t, err)
	defer resp.Body.Close()

	respByteArray, _ := ioutil.ReadAll(resp.Body)
	respString := string(respByteArray)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, expectedRespString, respString)
}
