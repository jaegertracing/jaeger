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

package esclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const es5PingResponse = `{
  "name" : "H_3P3Ll",
  "cluster_name" : "docker-cluster",
  "cluster_uuid" : "j-Kn4lBKTdCE1Cp9V4iYcA",
  "version" : {
    "number" : "5.6.10",
    "build_hash" : "b727a60",
    "build_date" : "2018-06-06T15:48:34.860Z",
    "build_snapshot" : false,
    "lucene_version" : "6.6.1"
  },
  "tagline" : "You Know, for Search"
}`
const es8PingResponse = `{
  "name" : "509984c472e3",
  "cluster_name" : "docker-cluster",
  "cluster_uuid" : "GJbBkpRLQZil3DiqGFUTDQ",
  "version" : {
    "number" : "8.0.0-SNAPSHOT",
    "build_flavor" : "oss",
    "build_type" : "docker",
    "build_hash" : "ebe89518795211eeba01b21c65d9396702441d0a",
    "build_date" : "2020-06-16T17:13:26.051209Z",
    "build_snapshot" : true,
    "lucene_version" : "8.6.0",
    "minimum_wire_compatibility_version" : "7.9.0",
    "minimum_index_compatibility_version" : "7.0.0"
  },
  "tagline" : "You Know, for Search"
}`

func TestPing(t *testing.T) {
	tests := []struct {
		name    string
		resp    string
		err     string
		version int
	}{
		{
			name:    "e5",
			resp:    es5PingResponse,
			version: 5,
		},
		{
			name:    "es8",
			resp:    es8PingResponse,
			version: 8,
		},
		{
			name: "wrong response",
			resp: "foo",
			err:  "invalid character 'o' in literal false (expecting 'a')",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ts := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Add("Content-Type", "application/json")
					w.Write([]byte(test.resp))
				}),
			)
			defer ts.Close()
			esPing := elasticsearchPing{}
			version, err := esPing.getVersion(ts.URL)
			if test.err != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.err)
				assert.Equal(t, 0, version)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.version, version)
			}
		})
	}
}
