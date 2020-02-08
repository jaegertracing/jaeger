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

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdditionalHeadersHandler(t *testing.T) {
	additionalHeaders := http.Header{}
	additionalHeaders.Add("Access-Control-Allow-Origin", "https://mozilla.org")
	additionalHeaders.Add("Access-Control-Expose-Headers", "X-My-Custom-Header")
	additionalHeaders.Add("Access-Control-Expose-Headers", "X-Another-Custom-Header")
	additionalHeaders.Add("Access-Control-Request-Headers", "field1, field2")

	emptyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{})
	})

	handler := additionalHeadersHandler(emptyHandler, additionalHeaders)
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL, nil)
	assert.NoError(t, err)

	resp, err := httpClient.Do(req)
	assert.NoError(t, err)

	for k, v := range additionalHeaders {
		assert.Equal(t, v, resp.Header[k])
	}
}
