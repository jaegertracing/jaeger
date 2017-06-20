// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package mocks

import "github.com/uber/jaeger/model"
import "github.com/stretchr/testify/mock"

import "time"

type Store struct {
	mock.Mock
}

func (_m *Store) InsertThroughput(throughput []*model.Throughput) error {
	ret := _m.Called(throughput)

	var r0 error
	if rf, ok := ret.Get(0).(func([]*model.Throughput) error); ok {
		r0 = rf(throughput)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
func (_m *Store) InsertProbabilitiesAndQPS(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS) error {
	ret := _m.Called(hostname, probabilities, qps)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, model.ServiceOperationProbabilities, model.ServiceOperationQPS) error); ok {
		r0 = rf(hostname, probabilities, qps)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
func (_m *Store) GetThroughput(start time.Time, end time.Time) ([]*model.Throughput, error) {
	ret := _m.Called(start, end)

	var r0 []*model.Throughput
	if rf, ok := ret.Get(0).(func(time.Time, time.Time) []*model.Throughput); ok {
		r0 = rf(start, end)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Throughput)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(time.Time, time.Time) error); ok {
		r1 = rf(start, end)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
func (_m *Store) GetProbabilitiesAndQPS(start time.Time, end time.Time) (map[string][]model.ServiceOperationData, error) {
	ret := _m.Called(start, end)

	var r0 map[string][]model.ServiceOperationData
	if rf, ok := ret.Get(0).(func(time.Time, time.Time) map[string][]model.ServiceOperationData); ok {
		r0 = rf(start, end)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string][]model.ServiceOperationData)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(time.Time, time.Time) error); ok {
		r1 = rf(start, end)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
func (_m *Store) GetLatestProbabilities() (model.ServiceOperationProbabilities, error) {
	ret := _m.Called()

	var r0 model.ServiceOperationProbabilities
	if rf, ok := ret.Get(0).(func() model.ServiceOperationProbabilities); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(model.ServiceOperationProbabilities)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
