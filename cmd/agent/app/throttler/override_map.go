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
	return &overrideMap{
		maxOverrides: maxOverrides,
		overrides:    map[string]interface{}{},
		defaultValue: defaultValue,
	}
}

// Has checks if the given key matches an override in the override map.
func (o *overrideMap) Has(key string) bool {
	_, ok := o.overrides[key]
	return ok
}

// IsFull checks if the map is at its capacity, meaning it cannot insert new
// overrides.
func (o *overrideMap) IsFull() bool {
	return len(o.overrides) >= o.maxOverrides
}

// Get returns the override value associated with the given key, and the default
// value if the key is not in the override map.
func (o *overrideMap) Get(key string) interface{} {
	if value, ok := o.overrides[key]; ok {
		return value
	}
	return o.defaultValue
}

// Set sets a key-value pair override in the map. Returns true if an override value
// was inserted or overwritten. Returns false if the map could not insert the
// value due to capacity constraints.
func (o *overrideMap) Set(key string, value interface{}) bool {
	if _, ok := o.overrides[key]; ok || (len(o.overrides) < o.maxOverrides) {
		o.overrides[key] = value
		return true
	}
	return false
}

// Default returns the current default value.
func (o *overrideMap) Default() interface{} {
	return o.defaultValue
}

// SetDefault overwrites the current default value with a new one.
func (o *overrideMap) SetDefault(defaultValue interface{}) {
	o.defaultValue = defaultValue
}

// Delete deletes an override with the given key from the map.
func (o *overrideMap) Delete(key string) {
	delete(o.overrides, key)
}

// UpdateMaxOverrides updates the maximum number of overrides with a new value.
// If the number of overrides exceeds the new limit, an arbitrary selection of
// overrides are dropped until the map size is at the new limit.
func (o *overrideMap) UpdateMaxOverrides(maxOverrides int) {
	if len(o.overrides) > maxOverrides {
		keys := o.Keys()
		for len(o.overrides) > maxOverrides {
			delete(o.overrides, keys[0])
			keys = keys[1:]
		}
	}
	o.maxOverrides = maxOverrides
}

// Keys returns a list of all the unique override keys in the override map.
func (o *overrideMap) Keys() []string {
	keys := make([]string, len(o.overrides))
	index := 0
	for key := range o.overrides {
		keys[index] = key
		index++
	}
	return keys
}
