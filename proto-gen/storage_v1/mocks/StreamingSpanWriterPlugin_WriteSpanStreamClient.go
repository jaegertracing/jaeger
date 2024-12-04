// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	metadata "google.golang.org/grpc/metadata"

	storage_v1 "github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)

// StreamingSpanWriterPlugin_WriteSpanStreamClient is an autogenerated mock type for the StreamingSpanWriterPlugin_WriteSpanStreamClient type
type StreamingSpanWriterPlugin_WriteSpanStreamClient struct {
	mock.Mock
}

// CloseAndRecv provides a mock function with no fields
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) CloseAndRecv() (*storage_v1.WriteSpanResponse, error) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CloseAndRecv")
	}

	var r0 *storage_v1.WriteSpanResponse
	var r1 error
	if rf, ok := ret.Get(0).(func() (*storage_v1.WriteSpanResponse, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() *storage_v1.WriteSpanResponse); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storage_v1.WriteSpanResponse)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CloseSend provides a mock function with no fields
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) CloseSend() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CloseSend")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Context provides a mock function with no fields
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) Context() context.Context {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Context")
	}

	var r0 context.Context
	if rf, ok := ret.Get(0).(func() context.Context); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(context.Context)
		}
	}

	return r0
}

// Header provides a mock function with no fields
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) Header() (metadata.MD, error) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Header")
	}

	var r0 metadata.MD
	var r1 error
	if rf, ok := ret.Get(0).(func() (metadata.MD, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() metadata.MD); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(metadata.MD)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RecvMsg provides a mock function with given fields: m
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) RecvMsg(m any) error {
	ret := _m.Called(m)

	if len(ret) == 0 {
		panic("no return value specified for RecvMsg")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(any) error); ok {
		r0 = rf(m)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Send provides a mock function with given fields: _a0
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) Send(_a0 *storage_v1.WriteSpanRequest) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for Send")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*storage_v1.WriteSpanRequest) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SendMsg provides a mock function with given fields: m
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) SendMsg(m any) error {
	ret := _m.Called(m)

	if len(ret) == 0 {
		panic("no return value specified for SendMsg")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(any) error); ok {
		r0 = rf(m)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Trailer provides a mock function with no fields
func (_m *StreamingSpanWriterPlugin_WriteSpanStreamClient) Trailer() metadata.MD {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Trailer")
	}

	var r0 metadata.MD
	if rf, ok := ret.Get(0).(func() metadata.MD); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(metadata.MD)
		}
	}

	return r0
}

// NewStreamingSpanWriterPlugin_WriteSpanStreamClient creates a new instance of StreamingSpanWriterPlugin_WriteSpanStreamClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewStreamingSpanWriterPlugin_WriteSpanStreamClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *StreamingSpanWriterPlugin_WriteSpanStreamClient {
	mock := &StreamingSpanWriterPlugin_WriteSpanStreamClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
