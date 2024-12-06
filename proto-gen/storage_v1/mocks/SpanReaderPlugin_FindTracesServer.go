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

// SpanReaderPlugin_FindTracesServer is an autogenerated mock type for the SpanReaderPlugin_FindTracesServer type
type SpanReaderPlugin_FindTracesServer struct {
	mock.Mock
}

// Context provides a mock function with no fields
func (_m *SpanReaderPlugin_FindTracesServer) Context() context.Context {
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

// RecvMsg provides a mock function with given fields: m
func (_m *SpanReaderPlugin_FindTracesServer) RecvMsg(m any) error {
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
func (_m *SpanReaderPlugin_FindTracesServer) Send(_a0 *storage_v1.SpansResponseChunk) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for Send")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*storage_v1.SpansResponseChunk) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SendHeader provides a mock function with given fields: _a0
func (_m *SpanReaderPlugin_FindTracesServer) SendHeader(_a0 metadata.MD) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for SendHeader")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(metadata.MD) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SendMsg provides a mock function with given fields: m
func (_m *SpanReaderPlugin_FindTracesServer) SendMsg(m any) error {
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

// SetHeader provides a mock function with given fields: _a0
func (_m *SpanReaderPlugin_FindTracesServer) SetHeader(_a0 metadata.MD) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for SetHeader")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(metadata.MD) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTrailer provides a mock function with given fields: _a0
func (_m *SpanReaderPlugin_FindTracesServer) SetTrailer(_a0 metadata.MD) {
	_m.Called(_a0)
}

// NewSpanReaderPlugin_FindTracesServer creates a new instance of SpanReaderPlugin_FindTracesServer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewSpanReaderPlugin_FindTracesServer(t interface {
	mock.TestingT
	Cleanup(func())
}) *SpanReaderPlugin_FindTracesServer {
	mock := &SpanReaderPlugin_FindTracesServer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
