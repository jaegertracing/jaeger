// Code generated by mockery; DO NOT EDIT.
// github.com/vektra/mockery
// template: testify
// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

package mocks

import (
	"github.com/jaegertracing/jaeger/internal/uimodel"
	mock "github.com/stretchr/testify/mock"
)

// NewCollectorService creates a new instance of CollectorService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewCollectorService(t interface {
	mock.TestingT
	Cleanup(func())
}) *CollectorService {
	mock := &CollectorService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

// CollectorService is an autogenerated mock type for the CollectorService type
type CollectorService struct {
	mock.Mock
}

type CollectorService_Expecter struct {
	mock *mock.Mock
}

func (_m *CollectorService) EXPECT() *CollectorService_Expecter {
	return &CollectorService_Expecter{mock: &_m.Mock}
}

// GetSamplingRate provides a mock function for the type CollectorService
func (_mock *CollectorService) GetSamplingRate(service string, operation string) (float64, error) {
	ret := _mock.Called(service, operation)

	if len(ret) == 0 {
		panic("no return value specified for GetSamplingRate")
	}

	var r0 float64
	var r1 error
	if returnFunc, ok := ret.Get(0).(func(string, string) (float64, error)); ok {
		return returnFunc(service, operation)
	}
	if returnFunc, ok := ret.Get(0).(func(string, string) float64); ok {
		r0 = returnFunc(service, operation)
	} else {
		r0 = ret.Get(0).(float64)
	}
	if returnFunc, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = returnFunc(service, operation)
	} else {
		r1 = ret.Error(1)
	}
	return r0, r1
}

// CollectorService_GetSamplingRate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetSamplingRate'
type CollectorService_GetSamplingRate_Call struct {
	*mock.Call
}

// GetSamplingRate is a helper method to define mock.On call
//   - service
//   - operation
func (_e *CollectorService_Expecter) GetSamplingRate(service interface{}, operation interface{}) *CollectorService_GetSamplingRate_Call {
	return &CollectorService_GetSamplingRate_Call{Call: _e.mock.On("GetSamplingRate", service, operation)}
}

func (_c *CollectorService_GetSamplingRate_Call) Run(run func(service string, operation string)) *CollectorService_GetSamplingRate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string))
	})
	return _c
}

func (_c *CollectorService_GetSamplingRate_Call) Return(f float64, err error) *CollectorService_GetSamplingRate_Call {
	_c.Call.Return(f, err)
	return _c
}

func (_c *CollectorService_GetSamplingRate_Call) RunAndReturn(run func(service string, operation string) (float64, error)) *CollectorService_GetSamplingRate_Call {
	_c.Call.Return(run)
	return _c
}

// NewQueryService creates a new instance of QueryService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewQueryService(t interface {
	mock.TestingT
	Cleanup(func())
}) *QueryService {
	mock := &QueryService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

// QueryService is an autogenerated mock type for the QueryService type
type QueryService struct {
	mock.Mock
}

type QueryService_Expecter struct {
	mock *mock.Mock
}

func (_m *QueryService) EXPECT() *QueryService_Expecter {
	return &QueryService_Expecter{mock: &_m.Mock}
}

// GetTraces provides a mock function for the type QueryService
func (_mock *QueryService) GetTraces(serviceName string, operation string, tags map[string]string) ([]*uimodel.Trace, error) {
	ret := _mock.Called(serviceName, operation, tags)

	if len(ret) == 0 {
		panic("no return value specified for GetTraces")
	}

	var r0 []*uimodel.Trace
	var r1 error
	if returnFunc, ok := ret.Get(0).(func(string, string, map[string]string) ([]*uimodel.Trace, error)); ok {
		return returnFunc(serviceName, operation, tags)
	}
	if returnFunc, ok := ret.Get(0).(func(string, string, map[string]string) []*uimodel.Trace); ok {
		r0 = returnFunc(serviceName, operation, tags)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*uimodel.Trace)
		}
	}
	if returnFunc, ok := ret.Get(1).(func(string, string, map[string]string) error); ok {
		r1 = returnFunc(serviceName, operation, tags)
	} else {
		r1 = ret.Error(1)
	}
	return r0, r1
}

// QueryService_GetTraces_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetTraces'
type QueryService_GetTraces_Call struct {
	*mock.Call
}

// GetTraces is a helper method to define mock.On call
//   - serviceName
//   - operation
//   - tags
func (_e *QueryService_Expecter) GetTraces(serviceName interface{}, operation interface{}, tags interface{}) *QueryService_GetTraces_Call {
	return &QueryService_GetTraces_Call{Call: _e.mock.On("GetTraces", serviceName, operation, tags)}
}

func (_c *QueryService_GetTraces_Call) Run(run func(serviceName string, operation string, tags map[string]string)) *QueryService_GetTraces_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string), args[2].(map[string]string))
	})
	return _c
}

func (_c *QueryService_GetTraces_Call) Return(traces []*uimodel.Trace, err error) *QueryService_GetTraces_Call {
	_c.Call.Return(traces, err)
	return _c
}

func (_c *QueryService_GetTraces_Call) RunAndReturn(run func(serviceName string, operation string, tags map[string]string) ([]*uimodel.Trace, error)) *QueryService_GetTraces_Call {
	_c.Call.Return(run)
	return _c
}
