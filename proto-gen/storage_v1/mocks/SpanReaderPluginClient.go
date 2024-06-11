// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	context "context"

	grpc "google.golang.org/grpc"

	mock "github.com/stretchr/testify/mock"

	storage_v1 "github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)

// SpanReaderPluginClient is an autogenerated mock type for the SpanReaderPluginClient type
type SpanReaderPluginClient struct {
	mock.Mock
}

// FindTraceIDs provides a mock function with given fields: ctx, in, opts
func (_m *SpanReaderPluginClient) FindTraceIDs(ctx context.Context, in *storage_v1.FindTraceIDsRequest, opts ...grpc.CallOption) (*storage_v1.FindTraceIDsResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for FindTraceIDs")
	}

	var r0 *storage_v1.FindTraceIDsResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.FindTraceIDsRequest, ...grpc.CallOption) (*storage_v1.FindTraceIDsResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.FindTraceIDsRequest, ...grpc.CallOption) *storage_v1.FindTraceIDsResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storage_v1.FindTraceIDsResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *storage_v1.FindTraceIDsRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FindTraces provides a mock function with given fields: ctx, in, opts
func (_m *SpanReaderPluginClient) FindTraces(ctx context.Context, in *storage_v1.FindTracesRequest, opts ...grpc.CallOption) (storage_v1.SpanReaderPlugin_FindTracesClient, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for FindTraces")
	}

	var r0 storage_v1.SpanReaderPlugin_FindTracesClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.FindTracesRequest, ...grpc.CallOption) (storage_v1.SpanReaderPlugin_FindTracesClient, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.FindTracesRequest, ...grpc.CallOption) storage_v1.SpanReaderPlugin_FindTracesClient); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(storage_v1.SpanReaderPlugin_FindTracesClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *storage_v1.FindTracesRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetOperations provides a mock function with given fields: ctx, in, opts
func (_m *SpanReaderPluginClient) GetOperations(ctx context.Context, in *storage_v1.GetOperationsRequest, opts ...grpc.CallOption) (*storage_v1.GetOperationsResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetOperations")
	}

	var r0 *storage_v1.GetOperationsResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.GetOperationsRequest, ...grpc.CallOption) (*storage_v1.GetOperationsResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.GetOperationsRequest, ...grpc.CallOption) *storage_v1.GetOperationsResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storage_v1.GetOperationsResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *storage_v1.GetOperationsRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetServices provides a mock function with given fields: ctx, in, opts
func (_m *SpanReaderPluginClient) GetServices(ctx context.Context, in *storage_v1.GetServicesRequest, opts ...grpc.CallOption) (*storage_v1.GetServicesResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetServices")
	}

	var r0 *storage_v1.GetServicesResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.GetServicesRequest, ...grpc.CallOption) (*storage_v1.GetServicesResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.GetServicesRequest, ...grpc.CallOption) *storage_v1.GetServicesResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*storage_v1.GetServicesResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *storage_v1.GetServicesRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTrace provides a mock function with given fields: ctx, in, opts
func (_m *SpanReaderPluginClient) GetTrace(ctx context.Context, in *storage_v1.GetTraceRequest, opts ...grpc.CallOption) (storage_v1.SpanReaderPlugin_GetTraceClient, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetTrace")
	}

	var r0 storage_v1.SpanReaderPlugin_GetTraceClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.GetTraceRequest, ...grpc.CallOption) (storage_v1.SpanReaderPlugin_GetTraceClient, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *storage_v1.GetTraceRequest, ...grpc.CallOption) storage_v1.SpanReaderPlugin_GetTraceClient); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(storage_v1.SpanReaderPlugin_GetTraceClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *storage_v1.GetTraceRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewSpanReaderPluginClient creates a new instance of SpanReaderPluginClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewSpanReaderPluginClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *SpanReaderPluginClient {
	mock := &SpanReaderPluginClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
