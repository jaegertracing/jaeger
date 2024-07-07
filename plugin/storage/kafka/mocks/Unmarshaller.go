// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	model "github.com/jaegertracing/jaeger/model"
	mock "github.com/stretchr/testify/mock"
)

// Unmarshaller is an autogenerated mock type for the Unmarshaller type
type Unmarshaller struct {
	mock.Mock
}

// Unmarshal provides a mock function with given fields: _a0
func (_m *Unmarshaller) Unmarshal(_a0 []byte) (*model.Span, error) {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for Unmarshal")
	}

	var r0 *model.Span
	var r1 error
	if rf, ok := ret.Get(0).(func([]byte) (*model.Span, error)); ok {
		return rf(_a0)
	}
	if rf, ok := ret.Get(0).(func([]byte) *model.Span); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Span)
		}
	}

	if rf, ok := ret.Get(1).(func([]byte) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewUnmarshaller creates a new instance of Unmarshaller. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewUnmarshaller(t interface {
	mock.TestingT
	Cleanup(func())
}) *Unmarshaller {
	mock := &Unmarshaller{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
