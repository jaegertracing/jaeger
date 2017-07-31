// Code generated by mockery v1.0.0
package mocks

import context "context"
import elastic "github.com/olivere/elastic"
import es "github.com/uber/jaeger/pkg/es"
import mock "github.com/stretchr/testify/mock"

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

	var r0 *elastic.MultiSearchResult
	if rf, ok := ret.Get(0).(func(context.Context) *elastic.MultiSearchResult); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*elastic.MultiSearchResult)
		}
	}

	var r1 error
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
