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
	handler, err := NewStaticAssetsHandler("fixture", "")
	require.NoError(t, err)
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
	_, err := NewStaticAssetsHandler("", "")
	assert.EqualError(t, err, "Cannot read UI static assets: open jaeger-ui-build/build/index.html: no such file or directory")
}

func TestRegisterRoutesHandler(t *testing.T) {
	r := mux.NewRouter()
	handler, err := NewStaticAssetsHandler("fixture/", "")
	require.NoError(t, err)
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

func TestLoadUIConfig(t *testing.T) {
	type testCase struct {
		configFile    string
		expected      map[string]interface{}
		expectedError string
	}

	run := func(description string, testCase testCase) {
		t.Run(description, func(t *testing.T) {
			config, err := loadUIConfig(testCase.configFile)
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, testCase.expected, config)
		})
	}

	run("no config", testCase{})
	run("invalid config", testCase{
		configFile:    "invalid",
		expectedError: "Cannot read UI config file invalid: open invalid: no such file or directory",
	})
	run("unsupported type", testCase{
		configFile:    "fixture/ui-config.toml",
		expectedError: "Unrecognized UI config file format fixture/ui-config.toml",
	})
	run("malformed", testCase{
		configFile:    "fixture/ui-config-malformed.json",
		expectedError: "Cannot parse UI config file fixture/ui-config-malformed.json: invalid character '=' after object key",
	})
	run("yaml", testCase{
		configFile: "fixture/ui-config.yaml",
		expected: map[string]interface{}{
			"x": "abcd",
			"z": []interface{}{"a", "b"},
		},
	})
	run("yml", testCase{
		configFile: "fixture/ui-config.yml",
		expected: map[string]interface{}{
			"x": "abcd",
			"z": []interface{}{"a", "b"},
		},
	})
	run("json", testCase{
		configFile: "fixture/ui-config.json",
		expected:   map[string]interface{}{"x": "y"},
	})
}
