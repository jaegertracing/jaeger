// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Run 'make generate-mocks' to regenerate.

// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	io "io"

	mock "github.com/stretchr/testify/mock"
)

// TemplateApplier is an autogenerated mock type for the TemplateApplier type
type TemplateApplier struct {
	mock.Mock
}

// Execute provides a mock function with given fields: wr, data
func (_m *TemplateApplier) Execute(wr io.Writer, data any) error {
	ret := _m.Called(wr, data)

	if len(ret) == 0 {
		panic("no return value specified for Execute")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(io.Writer, any) error); ok {
		r0 = rf(wr, data)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewTemplateApplier creates a new instance of TemplateApplier. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewTemplateApplier(t interface {
	mock.TestingT
	Cleanup(func())
}) *TemplateApplier {
	mock := &TemplateApplier{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
