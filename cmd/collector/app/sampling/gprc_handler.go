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
	"context"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// GRPCHandler is sampling strategy handler for gRPC.
type GRPCHandler struct {
	store strategystore.StrategyStore
}

// NewGRPCHandler creates a handler that controls sampling strategies for services.
func NewGRPCHandler(store strategystore.StrategyStore) GRPCHandler {
	return GRPCHandler{
		store: store,
	}
}

// GetSamplingStrategy returns sampling decision from store.
func (s GRPCHandler) GetSamplingStrategy(c context.Context, param *api_v2.SamplingStrategyParameters) (*api_v2.SamplingStrategyResponse, error) {
	r, err := s.store.GetSamplingStrategy(param.GetServiceName())
	if err != nil {
		return nil, err
	}
	return jaeger.ConvertSamplingResponseToDomain(r)
}
