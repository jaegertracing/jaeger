// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	context "context"

	dbmodel "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/dependencystore/dbmodel"

	mock "github.com/stretchr/testify/mock"

	time "time"
)

// CoreDependencyStore is an autogenerated mock type for the CoreDependencyStore type
type CoreDependencyStore struct {
	mock.Mock
}

// CreateTemplates provides a mock function with given fields: dependenciesTemplate
func (_m *CoreDependencyStore) CreateTemplates(dependenciesTemplate string) error {
	ret := _m.Called(dependenciesTemplate)

	if len(ret) == 0 {
		panic("no return value specified for CreateTemplates")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(dependenciesTemplate)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetDependencies provides a mock function with given fields: ctx, endTs, lookback
func (_m *CoreDependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]dbmodel.DependencyLink, error) {
	ret := _m.Called(ctx, endTs, lookback)

	if len(ret) == 0 {
		panic("no return value specified for GetDependencies")
	}

	var r0 []dbmodel.DependencyLink
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, time.Time, time.Duration) ([]dbmodel.DependencyLink, error)); ok {
		return rf(ctx, endTs, lookback)
	}
	if rf, ok := ret.Get(0).(func(context.Context, time.Time, time.Duration) []dbmodel.DependencyLink); ok {
		r0 = rf(ctx, endTs, lookback)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]dbmodel.DependencyLink)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, time.Time, time.Duration) error); ok {
		r1 = rf(ctx, endTs, lookback)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// WriteDependencies provides a mock function with given fields: ts, dependencies
func (_m *CoreDependencyStore) WriteDependencies(ts time.Time, dependencies []dbmodel.DependencyLink) error {
	ret := _m.Called(ts, dependencies)

	if len(ret) == 0 {
		panic("no return value specified for WriteDependencies")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Time, []dbmodel.DependencyLink) error); ok {
		r0 = rf(ts, dependencies)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewCoreDependencyStore creates a new instance of CoreDependencyStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewCoreDependencyStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *CoreDependencyStore {
	mock := &CoreDependencyStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
