// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package mocks

import (
	"github.com/crossdock/crossdock-go"
	"github.com/stretchr/testify/mock"
)

// T is an autogenerated mock type for the T type
type T struct {
	mock.Mock
}

// Behavior provides a mock function with given fields:
func (_m *T) Behavior() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Param provides a mock function with given fields: key
func (_m *T) Param(key string) string {
	ret := _m.Called(key)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(key)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Tag provides a mock function with given fields: key, value
func (_m *T) Tag(key string, value string) {
	_m.Called(key, value)
}

// Errorf provides a mock function with given fields: format, args
func (_m *T) Errorf(format string, args ...interface{}) {
	_m.Called(format, args)
}

// Skipf provides a mock function with given fields: format, args
func (_m *T) Skipf(format string, args ...interface{}) {
	_m.Called(format, args)
}

// Successf provides a mock function with given fields: format, args
func (_m *T) Successf(format string, args ...interface{}) {
	_m.Called(format, args)
}

// Fatalf provides a mock function with given fields: format, args
func (_m *T) Fatalf(format string, args ...interface{}) {
	_m.Called(format, args)
}

// FailNow provides a mock function with given fields:
func (_m *T) FailNow() {
	_m.Called()
}

// Put provides a mock function with given fields: status, output
func (_m *T) Put(status crossdock.Status, output string) {
	_m.Called(status, output)
}
