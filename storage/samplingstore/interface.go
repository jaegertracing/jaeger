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

package samplingstore

import (
	"time"

	"github.com/uber/jaeger/cmd/collector/app/sampling/model"
)

// Store writes and retrieves sampling data to and from storage.
type Store interface {
	// InsertThroughput inserts aggregated throughput for operations into storage.
	InsertThroughput(throughput []*model.Throughput) error

	// InsertProbabilitiesAndQPS inserts calculated sampling probabilities and measured qps into storage.
	InsertProbabilitiesAndQPS(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS) error

	// GetThroughput retrieves aggregated throughput for operations within a time range.
	GetThroughput(start, end time.Time) ([]*model.Throughput, error)

	// GetProbabilitiesAndQPS retrieves the sampling probabilities and measured qps per host within a time range.
	GetProbabilitiesAndQPS(start, end time.Time) (map[string][]model.ServiceOperationData, error)

	// GetLatestProbabilities retrieves the latest sampling probabilities.
	GetLatestProbabilities() (model.ServiceOperationProbabilities, error)
}
