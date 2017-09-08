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
