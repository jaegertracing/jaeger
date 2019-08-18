// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package testutils

import (
	"sync"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
)

func newBaggageRestrictionManager() *baggageRestrictionManager {
	return &baggageRestrictionManager{
		restrictions: make(map[string][]*baggage.BaggageRestriction),
	}
}

type baggageRestrictionManager struct {
	sync.Mutex

	restrictions map[string][]*baggage.BaggageRestriction
}

// GetBaggageRestrictions implements handler method of baggage.BaggageRestrictionManager
func (m *baggageRestrictionManager) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	m.Lock()
	defer m.Unlock()
	if restrictions, ok := m.restrictions[serviceName]; ok {
		return restrictions, nil
	}
	return nil, nil
}

// AddBaggageRestrictions registers baggage restrictions for a service
func (m *baggageRestrictionManager) AddBaggageRestrictions(service string, restrictions []*baggage.BaggageRestriction) {
	m.Lock()
	defer m.Unlock()
	m.restrictions[service] = restrictions
}
