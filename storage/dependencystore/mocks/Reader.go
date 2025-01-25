// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"

	model "github.com/jaegertracing/jaeger-idl/model/v1"

	time "time"
)

// Reader is an autogenerated mock type for the Reader type
type Reader struct {
	mock.Mock
}

// GetDependencies provides a mock function with given fields: ctx, endTs, lookback
func (_m *Reader) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	ret := _m.Called(ctx, endTs, lookback)

	if len(ret) == 0 {
		panic("no return value specified for GetDependencies")
	}

	var r0 []model.DependencyLink
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, time.Time, time.Duration) ([]model.DependencyLink, error)); ok {
		return rf(ctx, endTs, lookback)
	}
	if rf, ok := ret.Get(0).(func(context.Context, time.Time, time.Duration) []model.DependencyLink); ok {
		r0 = rf(ctx, endTs, lookback)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]model.DependencyLink)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, time.Time, time.Duration) error); ok {
		r1 = rf(ctx, endTs, lookback)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewReader creates a new instance of Reader. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewReader(t interface {
	mock.TestingT
	Cleanup(func())
}) *Reader {
	mock := &Reader{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
