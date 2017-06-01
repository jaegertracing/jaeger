// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package testutils

import (
	"sync"

	"github.com/uber/jaeger/thrift-gen/baggage"
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
