// Copyright (c) 2020 The Jaeger Authors.
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

package status

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"status\":\"Server available\"}"))
}

func unavailableHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("{\"status\":\"Server not available\"}"))
}

func TestReady(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(readyHandler))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=" + strings.TrimPrefix(ts.URL, "http://")})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestOnlyPortConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(readyHandler))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=:" + strings.Split(ts.URL, ":")[len(strings.Split(ts.URL, ":"))-1]})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestUnready(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(unavailableHandler))
	defer ts.Close()
	v := viper.New()
	cmd := Command(v, 80)
	cmd.ParseFlags([]string{"--status.http.host-port=" + strings.TrimPrefix(ts.URL, "http://")})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestNoService(t *testing.T) {
	v := viper.New()
	cmd := Command(v, 12345)
	err := cmd.Execute()
	assert.Error(t, err)
}
