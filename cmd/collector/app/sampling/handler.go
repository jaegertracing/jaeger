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

package sampling

import (
	"github.com/uber/tchannel-go/thrift"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// Handler returns sampling strategies for specified services
type Handler interface {
	// GetSamplingStrategy returns recommended sampling strategy for a given service name.
	GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error)
}

type handler struct {
	store strategystore.StrategyStore
}

// NewHandler creates a handler that controls sampling strategies for services.
func NewHandler(store strategystore.StrategyStore) Handler {
	return &handler{
		store: store,
	}
}

// GetSamplingStrategy returns allowed sampling strategy for a given service name.
func (h *handler) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return h.store.GetSamplingStrategy(serviceName)
}
