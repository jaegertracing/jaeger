// Code generated by mockery v2.42.3. DO NOT EDIT.

package mocks

import (
	context "context"

	es "github.com/jaegertracing/jaeger/pkg/es"
	elastic "github.com/olivere/elastic"

	mock "github.com/stretchr/testify/mock"
)

// MultiSearchService is an autogenerated mock type for the MultiSearchService type
type MultiSearchService struct {
	mock.Mock
}

// Add provides a mock function with given fields: requests
func (_m *MultiSearchService) Add(requests ...*elastic.SearchRequest) es.MultiSearchService {
	_va := make([]interface{}, len(requests))
	for _i := range requests {
		_va[_i] = requests[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Add")
	}

	var r0 es.MultiSearchService
	if rf, ok := ret.Get(0).(func(...*elastic.SearchRequest) es.MultiSearchService); ok {
		r0 = rf(requests...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.MultiSearchService)
		}
	}

	return r0
}

// Do provides a mock function with given fields: ctx
func (_m *MultiSearchService) Do(ctx context.Context) (*elastic.MultiSearchResult, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Do")
	}

	var r0 *elastic.MultiSearchResult
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (*elastic.MultiSearchResult, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) *elastic.MultiSearchResult); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*elastic.MultiSearchResult)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Index provides a mock function with given fields: indices
func (_m *MultiSearchService) Index(indices ...string) es.MultiSearchService {
	_va := make([]interface{}, len(indices))
	for _i := range indices {
		_va[_i] = indices[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for Index")
	}

	var r0 es.MultiSearchService
	if rf, ok := ret.Get(0).(func(...string) es.MultiSearchService); ok {
		r0 = rf(indices...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.MultiSearchService)
		}
	}

	return r0
}

// NewMultiSearchService creates a new instance of MultiSearchService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMultiSearchService(t interface {
	mock.TestingT
	Cleanup(func())
}) *MultiSearchService {
	mock := &MultiSearchService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
