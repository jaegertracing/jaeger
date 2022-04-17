// Copyright (c) 2018 The Jaeger Authors.
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

package strategystore

import (
	"context"
	"io"

	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// StrategyStore keeps track of service specific sampling strategies.
type StrategyStore interface {
	// GetSamplingStrategy retrieves the sampling strategy for the specified service.
	GetSamplingStrategy(ctx context.Context, serviceName string) (*sampling.SamplingStrategyResponse, error)
}

// Aggregator defines an interface used to aggregate operation throughput.
type Aggregator interface {
	// Close() from io.Closer stops the aggregator from aggregating throughput.
	io.Closer

	// RecordThroughput records throughput for an operation for aggregation.
	RecordThroughput(service, operation, samplerType string, probability float64)

	// Start starts aggregating operation throughput.
	Start()
}
