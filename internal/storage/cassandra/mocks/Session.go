// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	cassandra "github.com/jaegertracing/jaeger/internal/storage/cassandra"
	mock "github.com/stretchr/testify/mock"
)

// Session is an autogenerated mock type for the Session type
type Session struct {
	mock.Mock
}

type Session_Expecter struct {
	mock *mock.Mock
}

func (_m *Session) EXPECT() *Session_Expecter {
	return &Session_Expecter{mock: &_m.Mock}
}

// Close provides a mock function with no fields
func (_m *Session) Close() {
	_m.Called()
}

// Session_Close_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Close'
type Session_Close_Call struct {
	*mock.Call
}

// Close is a helper method to define mock.On call
func (_e *Session_Expecter) Close() *Session_Close_Call {
	return &Session_Close_Call{Call: _e.mock.On("Close")}
}

func (_c *Session_Close_Call) Run(run func()) *Session_Close_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Session_Close_Call) Return() *Session_Close_Call {
	_c.Call.Return()
	return _c
}

func (_c *Session_Close_Call) RunAndReturn(run func()) *Session_Close_Call {
	_c.Run(run)
	return _c
}

// Query provides a mock function with given fields: stmt, values
func (_m *Session) Query(stmt string, values ...any) cassandra.Query {
	var tmpRet mock.Arguments
	if len(values) > 0 {
		tmpRet = _m.Called(stmt, values)
	} else {
		tmpRet = _m.Called(stmt)
	}
	ret := tmpRet

	if len(ret) == 0 {
		panic("no return value specified for Query")
	}

	var r0 cassandra.Query
	if rf, ok := ret.Get(0).(func(string, ...any) cassandra.Query); ok {
		r0 = rf(stmt, values...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(cassandra.Query)
		}
	}

	return r0
}

// Session_Query_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Query'
type Session_Query_Call struct {
	*mock.Call
}

// Query is a helper method to define mock.On call
//   - stmt string
//   - values ...any
func (_e *Session_Expecter) Query(stmt interface{}, values ...interface{}) *Session_Query_Call {
	return &Session_Query_Call{Call: _e.mock.On("Query",
		append([]interface{}{stmt}, values...)...)}
}

func (_c *Session_Query_Call) Run(run func(stmt string, values ...any)) *Session_Query_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]any, len(args)-1)
		for i, a := range args[1:] {
			if a != nil {
				variadicArgs[i] = a.(any)
			}
		}
		run(args[0].(string), variadicArgs...)
	})
	return _c
}

func (_c *Session_Query_Call) Return(_a0 cassandra.Query) *Session_Query_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Session_Query_Call) RunAndReturn(run func(string, ...any) cassandra.Query) *Session_Query_Call {
	_c.Call.Return(run)
	return _c
}

// NewSession creates a new instance of Session. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewSession(t interface {
	mock.TestingT
	Cleanup(func())
}) *Session {
	mock := &Session{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
