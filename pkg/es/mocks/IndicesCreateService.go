// Code generated by mockery v2.9.4. DO NOT EDIT.

// Copyright (c) 2021 The Jaeger Authors.
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

	elastic "github.com/olivere/elastic"
	mock "github.com/stretchr/testify/mock"

	es "github.com/jaegertracing/jaeger/pkg/es"
)

// IndicesCreateService is an autogenerated mock type for the IndicesCreateService type
type IndicesCreateService struct {
	mock.Mock
}

// Body provides a mock function with given fields: mapping
func (_m *IndicesCreateService) Body(mapping string) es.IndicesCreateService {
	ret := _m.Called(mapping)

	var r0 es.IndicesCreateService
	if rf, ok := ret.Get(0).(func(string) es.IndicesCreateService); ok {
		r0 = rf(mapping)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(es.IndicesCreateService)
		}
	}

	return r0
}

// Do provides a mock function with given fields: ctx
func (_m *IndicesCreateService) Do(ctx context.Context) (*elastic.IndicesCreateResult, error) {
	ret := _m.Called(ctx)

	var r0 *elastic.IndicesCreateResult
	if rf, ok := ret.Get(0).(func(context.Context) *elastic.IndicesCreateResult); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*elastic.IndicesCreateResult)
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
