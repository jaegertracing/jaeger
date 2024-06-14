// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	context "context"

	storage_v1 "github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	mock "github.com/stretchr/testify/mock"
)

// PluginCapabilitiesServer is an autogenerated mock type for the PluginCapabilitiesServer type
type PluginCapabilitiesServer struct {
	mock.Mock
}

// Capabilities provides a mock function with given fields: _a0, _a1
func (_m *PluginCapabilitiesServer) Capabilities(_a0 context.Context, _a1 *storage_v1.CapabilitiesRequest) (*storage_v1.CapabilitiesResponse, error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for Capabilities")
	}

	var r0 *storage_v1.CapabilitiesResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.CapabilitiesRequest) (*storage_v1.CapabilitiesResponse, error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.CapabilitiesRequest) *storage_v1.CapabilitiesResponse); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storage_v1.CapabilitiesResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *storage_v1.CapabilitiesRequest) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewPluginCapabilitiesServer creates a new instance of PluginCapabilitiesServer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewPluginCapabilitiesServer(t interface {
	mock.TestingT
	Cleanup(func())
}) *PluginCapabilitiesServer {
	mock := &PluginCapabilitiesServer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
