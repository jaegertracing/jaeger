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

// SamplingCacheEntry keeps track of the probability and whether a service-operation is observed
// using adaptive sampling.
type SamplingCacheEntry struct {
	Probability   float64
	UsingAdaptive bool
}

// SamplingCache is a nested map: service -> operation -> cache entry.
type SamplingCache map[string]map[string]*SamplingCacheEntry

// Set adds a new entry for given service/operation.
func (s SamplingCache) Set(service, operation string, entry *SamplingCacheEntry) {
	if _, ok := s[service]; !ok {
		s[service] = make(map[string]*SamplingCacheEntry)
	}
	s[service][operation] = entry
}

// Get retrieves the entry for given service/operation. Returns nil if not found.
func (s SamplingCache) Get(service, operation string) *SamplingCacheEntry {
	v, ok := s[service]
	if !ok {
		return nil
	}
	return v[operation]
}
