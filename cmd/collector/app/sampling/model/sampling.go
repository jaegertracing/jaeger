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
