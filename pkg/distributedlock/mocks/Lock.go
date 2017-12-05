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

package mocks

import (
	"time"

	"github.com/stretchr/testify/mock"
)

// Lock mocks distributed lock for control of a resource.
type Lock struct {
	mock.Mock
}

// Acquire acquires a lease of duration ttl around a given resource. In case of an error,
// acquired is meaningless.
func (_m *Lock) Acquire(resource string, ttl time.Duration) (bool, error) {
	ret := _m.Called(resource, ttl)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string, time.Duration) bool); ok {
		r0 = rf(resource, ttl)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, time.Duration) error); ok {
		r1 = rf(resource, ttl)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Forfeit forfeits a lease around a given resource. In case of an error,
// forfeited is meaningless.
func (_m *Lock) Forfeit(resource string) (bool, error) {
	ret := _m.Called(resource)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string) bool); ok {
		r0 = rf(resource)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(resource)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
