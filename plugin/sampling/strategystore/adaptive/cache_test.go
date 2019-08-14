// Copyright (c) 2018 The Jaeger Authors.
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

package adaptive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSamplingCache(t *testing.T) {
	var (
		c         = SamplingCache{}
		service   = "svc"
		operation = "op"
	)
	c.Set(service, operation, &SamplingCacheEntry{})
	assert.NotNil(t, c.Get(service, operation))
	assert.Nil(t, c.Get("blah", "blah"))
}
