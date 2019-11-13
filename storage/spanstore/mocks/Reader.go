// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mocks

import (
	context "context"

	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)
import mock "github.com/stretchr/testify/mock"
import model "github.com/jaegertracing/jaeger/model"
import spanstore "github.com/jaegertracing/jaeger/storage/spanstore"

// Reader is an autogenerated mock type for the Reader type
type Reader struct {
	mock.Mock
}

// FindTraceIDs provides a mock function with given fields: ctx, query
func (_m *Reader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ret := _m.Called(ctx, query)

	var r0 []model.TraceID
	if rf, ok := ret.Get(0).(func(context.Context, *spanstore.TraceQueryParameters) []model.TraceID); ok {
		r0 = rf(ctx, query)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]model.TraceID)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *spanstore.TraceQueryParameters) error); ok {
		r1 = rf(ctx, query)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FindTraces provides a mock function with given fields: ctx, query
func (_m *Reader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ret := _m.Called(ctx, query)

	var r0 []*model.Trace
	if rf, ok := ret.Get(0).(func(context.Context, *spanstore.TraceQueryParameters) []*model.Trace); ok {
		r0 = rf(ctx, query)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Trace)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *spanstore.TraceQueryParameters) error); ok {
		r1 = rf(ctx, query)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetOperations provides a mock function with given fields: ctx, query
func (_m *Reader) GetOperations(ctx context.Context, query *spanstore.OperationQueryParameters) ([]*storage_v1.Operation, error) {
	ret := _m.Called(ctx, query)

	var r0 []*storage_v1.Operation
	if rf, ok := ret.Get(0).(func(context.Context, *spanstore.OperationQueryParameters) []*storage_v1.Operation); ok {
		r0 = rf(ctx, query)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*storage_v1.Operation)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *spanstore.OperationQueryParameters) error); ok {
		r1 = rf(ctx, query)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetServices provides a mock function with given fields: ctx
func (_m *Reader) GetServices(ctx context.Context) ([]string, error) {
	ret := _m.Called(ctx)

	var r0 []string
	if rf, ok := ret.Get(0).(func(context.Context) []string); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
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

// GetTrace provides a mock function with given fields: ctx, traceID
func (_m *Reader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	ret := _m.Called(ctx, traceID)

	var r0 *model.Trace
	if rf, ok := ret.Get(0).(func(context.Context, model.TraceID) *model.Trace); ok {
		r0 = rf(ctx, traceID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Trace)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, model.TraceID) error); ok {
		r1 = rf(ctx, traceID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
