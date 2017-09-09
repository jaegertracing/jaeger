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

import "github.com/uber/jaeger/cmd/collector/app/sampling/model"
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
