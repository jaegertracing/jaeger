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

// samplingCacheEntry keeps track of the probability and whether a service-operation is observed
// using adaptive sampling.
type samplingCacheEntry struct {
	probability   float64
	usingAdaptive bool
}

// nested map: service -> operation -> cache entry.
type samplingCache map[string]map[string]*samplingCacheEntry

func (s samplingCache) Set(service, operation string, entry *samplingCacheEntry) {
	if _, ok := s[service]; !ok {
		s[service] = make(map[string]*samplingCacheEntry)
	}
	s[service][operation] = entry
}

func (s samplingCache) Get(service, operation string) *samplingCacheEntry {
	v, ok := s[service]
	if !ok {
		return nil
	}
	return v[operation]
}
