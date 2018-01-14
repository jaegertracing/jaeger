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

// overrideMap describes a configuration map with a fixed number of override
// entries, falling back on a default value.
type overrideMap struct {
	maxOverrides int
	overrides    map[string]interface{}
	defaultValue interface{}
}

// newOverrideMap creates a new OverrideMap with maxOverrides overrides and a
// fallback value of defaultValue.
func newOverrideMap(maxOverrides int, defaultValue interface{}) *overrideMap {
	o := &overrideMap{
		maxOverrides: maxOverrides,
		overrides:    map[string]interface{}{},
		defaultValue: defaultValue,
	}
	return o
}

func (o *overrideMap) Has(key string) bool {
	_, ok := o.overrides[key]
	return ok
}

func (o *overrideMap) IsFull() bool {
	return len(o.overrides) >= o.maxOverrides
}

func (o *overrideMap) Get(key string) interface{} {
	if value, ok := o.overrides[key]; ok {
		return value
	}
	return o.defaultValue
}

func (o *overrideMap) Set(key string, value interface{}) {
	if _, ok := o.overrides[key]; !ok && len(o.overrides) >= o.maxOverrides {
		o.defaultValue = value
	} else {
		o.overrides[key] = value
	}
}

func (o *overrideMap) Delete(key string) {
	delete(o.overrides, key)
}
