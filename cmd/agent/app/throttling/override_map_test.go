// Copyright (c) 2018 Uber Technologies, Inc.
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

package throttling

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverrideMap(t *testing.T) {
	maxOverrides := 1
	defaultValue := 0.0
	o := NewOverrideMap(maxOverrides, defaultValue)
	o.Set("a", 1.0)
	o.Set("b", 2.0)
	assert.Equal(t, 1.0, o.Get("a").(float64))
	assert.Equal(t, 2.0, o.Get("b").(float64))
	assert.Equal(t, 2.0, o.Get("c").(float64))
	assert.True(t, o.IsFull())
	assert.True(t, o.Has("a"))
	o.Delete("a")
	assert.Equal(t, 2.0, o.Get("a").(float64))
	assert.False(t, o.IsFull())
	assert.False(t, o.Has("a"))
}
