// Copyright (c) 2023 The Jaeger Authors.
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

package zipkindesermocks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Fake test to provide test coverage.
func TestMocks(t *testing.T) {
	s := CreateEndpoint("serviceName", "ipv4", "ipv6", 1)
	assert.Equal(t, `{"serviceName": "serviceName", "ipv4": "ipv4", "ipv6": "ipv6", "port": 1}`, s)

	s = CreateAnno("val", 100, `{"endpoint": "x"}`)
	assert.Equal(t, `{"value": "val", "timestamp": 100, "endpoint": {"endpoint": "x"}}`, s)

	s = CreateBinAnno("key", "val", `{"endpoint": "x"}`)
	assert.Equal(t, `{"key": "key", "value": "val", "endpoint": {"endpoint": "x"}}`, s)

	s = CreateSpan("name", "id", "pid", "tid", 100, 100, false, "anno", "binAnno")
	assert.Equal(t, `[{"name": "name", "id": "id", "parentId": "pid", "traceId": "tid", "timestamp": 100, "duration": 100, "debug": false, "annotations": [anno], "binaryAnnotations": [binAnno]}]`, s)
}
