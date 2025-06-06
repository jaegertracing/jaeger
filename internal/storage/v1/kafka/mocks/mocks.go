// Code generated by mockery; DO NOT EDIT.
// github.com/vektra/mockery
// template: testify
// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

package mocks

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
	mock "github.com/stretchr/testify/mock"
)

// NewMarshaller creates a new instance of Marshaller. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMarshaller(t interface {
	mock.TestingT
	Cleanup(func())
}) *Marshaller {
	mock := &Marshaller{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

// Marshaller is an autogenerated mock type for the Marshaller type
type Marshaller struct {
	mock.Mock
}

type Marshaller_Expecter struct {
	mock *mock.Mock
}

func (_m *Marshaller) EXPECT() *Marshaller_Expecter {
	return &Marshaller_Expecter{mock: &_m.Mock}
}

// Marshal provides a mock function for the type Marshaller
func (_mock *Marshaller) Marshal(span *model.Span) ([]byte, error) {
	ret := _mock.Called(span)

	if len(ret) == 0 {
		panic("no return value specified for Marshal")
	}

	var r0 []byte
	var r1 error
	if returnFunc, ok := ret.Get(0).(func(*model.Span) ([]byte, error)); ok {
		return returnFunc(span)
	}
	if returnFunc, ok := ret.Get(0).(func(*model.Span) []byte); ok {
		r0 = returnFunc(span)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}
	if returnFunc, ok := ret.Get(1).(func(*model.Span) error); ok {
		r1 = returnFunc(span)
	} else {
		r1 = ret.Error(1)
	}
	return r0, r1
}

// Marshaller_Marshal_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Marshal'
type Marshaller_Marshal_Call struct {
	*mock.Call
}

// Marshal is a helper method to define mock.On call
//   - span *model.Span
func (_e *Marshaller_Expecter) Marshal(span interface{}) *Marshaller_Marshal_Call {
	return &Marshaller_Marshal_Call{Call: _e.mock.On("Marshal", span)}
}

func (_c *Marshaller_Marshal_Call) Run(run func(span *model.Span)) *Marshaller_Marshal_Call {
	_c.Call.Run(func(args mock.Arguments) {
		var arg0 *model.Span
		if args[0] != nil {
			arg0 = args[0].(*model.Span)
		}
		run(
			arg0,
		)
	})
	return _c
}

func (_c *Marshaller_Marshal_Call) Return(bytes []byte, err error) *Marshaller_Marshal_Call {
	_c.Call.Return(bytes, err)
	return _c
}

func (_c *Marshaller_Marshal_Call) RunAndReturn(run func(span *model.Span) ([]byte, error)) *Marshaller_Marshal_Call {
	_c.Call.Return(run)
	return _c
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

// Unmarshaller is an autogenerated mock type for the Unmarshaller type
type Unmarshaller struct {
	mock.Mock
}

type Unmarshaller_Expecter struct {
	mock *mock.Mock
}

func (_m *Unmarshaller) EXPECT() *Unmarshaller_Expecter {
	return &Unmarshaller_Expecter{mock: &_m.Mock}
}

// Unmarshal provides a mock function for the type Unmarshaller
func (_mock *Unmarshaller) Unmarshal(bytes []byte) (*model.Span, error) {
	ret := _mock.Called(bytes)

	if len(ret) == 0 {
		panic("no return value specified for Unmarshal")
	}

	var r0 *model.Span
	var r1 error
	if returnFunc, ok := ret.Get(0).(func([]byte) (*model.Span, error)); ok {
		return returnFunc(bytes)
	}
	if returnFunc, ok := ret.Get(0).(func([]byte) *model.Span); ok {
		r0 = returnFunc(bytes)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Span)
		}
	}
	if returnFunc, ok := ret.Get(1).(func([]byte) error); ok {
		r1 = returnFunc(bytes)
	} else {
		r1 = ret.Error(1)
	}
	return r0, r1
}

// Unmarshaller_Unmarshal_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Unmarshal'
type Unmarshaller_Unmarshal_Call struct {
	*mock.Call
}

// Unmarshal is a helper method to define mock.On call
//   - bytes []byte
func (_e *Unmarshaller_Expecter) Unmarshal(bytes interface{}) *Unmarshaller_Unmarshal_Call {
	return &Unmarshaller_Unmarshal_Call{Call: _e.mock.On("Unmarshal", bytes)}
}

func (_c *Unmarshaller_Unmarshal_Call) Run(run func(bytes []byte)) *Unmarshaller_Unmarshal_Call {
	_c.Call.Run(func(args mock.Arguments) {
		var arg0 []byte
		if args[0] != nil {
			arg0 = args[0].([]byte)
		}
		run(
			arg0,
		)
	})
	return _c
}

func (_c *Unmarshaller_Unmarshal_Call) Return(span *model.Span, err error) *Unmarshaller_Unmarshal_Call {
	_c.Call.Return(span, err)
	return _c
}

func (_c *Unmarshaller_Unmarshal_Call) RunAndReturn(run func(bytes []byte) (*model.Span, error)) *Unmarshaller_Unmarshal_Call {
	_c.Call.Return(run)
	return _c
}
