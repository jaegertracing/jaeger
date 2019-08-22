// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	es "github.com/jaegertracing/jaeger/pkg/es"
	mock "github.com/stretchr/testify/mock"
)

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Close provides a mock function with given fields:
func (_m *Client) Close() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateIndex provides a mock function with given fields: index
func (_m *Client) CreateIndex(index string) es.IndicesCreateService {
	ret := _m.Called(index)

	var r0 es.IndicesCreateService
	if rf, ok := ret.Get(0).(func(string) es.IndicesCreateService); ok {
		r0 = rf(index)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.IndicesCreateService)
		}
	}

	return r0
}

// CreateTemplate provides a mock function with given fields: id
func (_m *Client) CreateTemplate(id string) es.TemplateCreateService {
	ret := _m.Called(id)

	var r0 es.TemplateCreateService
	if rf, ok := ret.Get(0).(func(string) es.TemplateCreateService); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.TemplateCreateService)
		}
	}

	return r0
}

// GetVersion provides a mock function with given fields:
func (_m *Client) GetVersion() uint {
	ret := _m.Called()

	var r0 uint
	if rf, ok := ret.Get(0).(func() uint); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint)
	}

	return r0
}

// Index provides a mock function with given fields:
func (_m *Client) Index() es.IndexService {
	ret := _m.Called()

	var r0 es.IndexService
	if rf, ok := ret.Get(0).(func() es.IndexService); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.IndexService)
		}
	}

	return r0
}

// IndexExists provides a mock function with given fields: index
func (_m *Client) IndexExists(index string) es.IndicesExistsService {
	ret := _m.Called(index)

	var r0 es.IndicesExistsService
	if rf, ok := ret.Get(0).(func(string) es.IndicesExistsService); ok {
		r0 = rf(index)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.IndicesExistsService)
		}
	}

	return r0
}

// MultiSearch provides a mock function with given fields:
func (_m *Client) MultiSearch() es.MultiSearchService {
	ret := _m.Called()

	var r0 es.MultiSearchService
	if rf, ok := ret.Get(0).(func() es.MultiSearchService); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.MultiSearchService)
		}
	}

	return r0
}

// Search provides a mock function with given fields: indices
func (_m *Client) Search(indices ...string) es.SearchService {
	_va := make([]interface{}, len(indices))
	for _i := range indices {
		_va[_i] = indices[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 es.SearchService
	if rf, ok := ret.Get(0).(func(...string) es.SearchService); ok {
		r0 = rf(indices...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.SearchService)
		}
	}

	return r0
}
