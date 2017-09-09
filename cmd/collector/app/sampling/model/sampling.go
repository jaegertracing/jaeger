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

package model

// Throughput keeps track of the queries an operation received.
type Throughput struct {
	Service       string
	Operation     string
	Count         int64
	Probabilities map[string]struct{}
}

// ServiceOperationProbabilities contains the sampling probabilities for all operations in a service.
// ie [service][operation] = probability
type ServiceOperationProbabilities map[string]map[string]float64

// ServiceOperationQPS contains the qps for all operations in a service.
// ie [service][operation] = qps
type ServiceOperationQPS map[string]map[string]float64

// ProbabilityAndQPS contains the sampling probability and measured qps for an operation.
type ProbabilityAndQPS struct {
	Probability float64
	QPS         float64
}

// ServiceOperationData contains the sampling probabilities and measured qps for all operations in a service.
// ie [service][operation] = ProbabilityAndQPS
type ServiceOperationData map[string]map[string]*ProbabilityAndQPS
