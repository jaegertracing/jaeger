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
	"bytes"
	"encoding/json"
	"io"
)

// Elasticsearch header for multi search API
// https://www.elastic.co/guide/en/elasticsearch/reference/current/search-multi-search.html
const multiSearchHeaderFormat = `{"ignore_unavailable": "true"}` + "\n"

func encodeSearchBody(searchBody SearchBody) (io.Reader, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(searchBody); err != nil {
		return nil, err
	}
	return buf, nil
}

func encodeSearchBodies(searchBodies []SearchBody) (io.Reader, error) {
	buf := &bytes.Buffer{}
	for _, sb := range searchBodies {
		data, err := json.Marshal(sb)
		if err != nil {
			return nil, err
		}
		addDataToMSearchBuffer(buf, data)
	}
	return buf, nil
}

func addDataToMSearchBuffer(buffer *bytes.Buffer, data []byte) {
	meta := []byte(multiSearchHeaderFormat)
	buffer.Grow(len(data) + len(meta) + len("\n"))
	buffer.Write(meta)
	buffer.Write(data)
	buffer.Write([]byte("\n"))
}
