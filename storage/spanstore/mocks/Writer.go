// Code generated by mockery v2.10.4. DO NOT EDIT.

// Copyright (c) 2022 The Jaeger Authors.
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

	mock "github.com/stretchr/testify/mock"

	model "github.com/jaegertracing/jaeger/model"
)

// Writer is an autogenerated mock type for the Writer type
type Writer struct {
	mock.Mock
}

// WriteSpan provides a mock function with given fields: ctx, span
func (_m *Writer) WriteSpan(ctx context.Context, span *model.Span) error {
	ret := _m.Called(ctx, span)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *model.Span) error); ok {
		r0 = rf(ctx, span)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
