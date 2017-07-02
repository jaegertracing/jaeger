// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
	assert.Equal(t, "fixture/", handler.staticAssetsRoot)
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
