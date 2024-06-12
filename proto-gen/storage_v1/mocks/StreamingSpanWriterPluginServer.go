// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	storage_v1 "github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	mock "github.com/stretchr/testify/mock"
)

// StreamingSpanWriterPluginServer is an autogenerated mock type for the StreamingSpanWriterPluginServer type
type StreamingSpanWriterPluginServer struct {
	mock.Mock
}

// WriteSpanStream provides a mock function with given fields: _a0
func (_m *StreamingSpanWriterPluginServer) WriteSpanStream(_a0 storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamServer) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for WriteSpanStream")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamServer) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewStreamingSpanWriterPluginServer creates a new instance of StreamingSpanWriterPluginServer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewStreamingSpanWriterPluginServer(t interface {
	mock.TestingT
	Cleanup(func())
}) *StreamingSpanWriterPluginServer {
	mock := &StreamingSpanWriterPluginServer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
