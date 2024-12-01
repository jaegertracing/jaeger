// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	metrics "github.com/jaegertracing/jaeger/pkg/metrics"
	metricsstore "github.com/jaegertracing/jaeger/storage/metricsstore"

	mock "github.com/stretchr/testify/mock"

	zap "go.uber.org/zap"
)

// MetricsFactory is an autogenerated mock type for the MetricsFactory type
type MetricsFactory struct {
	mock.Mock
}

// CreateMetricsReader provides a mock function with given fields:
func (_m *MetricsFactory) CreateMetricsReader() (metricsstore.Reader, error) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CreateMetricsReader")
	}

	var r0 metricsstore.Reader
	var r1 error
	if rf, ok := ret.Get(0).(func() (metricsstore.Reader, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() metricsstore.Reader); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(metricsstore.Reader)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Initialize provides a mock function with given fields: metricsFactory, logger
func (_m *MetricsFactory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	ret := _m.Called(metricsFactory, logger)

	if len(ret) == 0 {
		panic("no return value specified for Initialize")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(metrics.Factory, *zap.Logger) error); ok {
		r0 = rf(metricsFactory, logger)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewMetricsFactory creates a new instance of MetricsFactory. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMetricsFactory(t interface {
	mock.TestingT
	Cleanup(func())
}) *MetricsFactory {
	mock := &MetricsFactory{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
