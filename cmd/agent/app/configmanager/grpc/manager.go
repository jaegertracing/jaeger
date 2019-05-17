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

package grpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	"github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type collectorProxy struct {
	samplingClient api_v2.SamplingManagerClient
	baggageClient  api_v2.BaggageServiceClient
}

// NewConfigManager implements Manager by proxying the requests to collector.
func NewConfigManager(conn *grpc.ClientConn) configmanager.ClientConfigManager {
	return &collectorProxy{
		samplingClient: api_v2.NewSamplingManagerClient(conn),
		baggageClient:  api_v2.NewBaggageServiceClient(conn),
	}
}

// GetSamplingStrategy returns sampling strategies from collector.
func (s *collectorProxy) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	r, err := s.samplingClient.GetSamplingStrategy(context.Background(), &api_v2.SamplingStrategyParameters{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}
	return jaeger.ConvertSamplingResponseFromDomain(r)
}

// GetBaggageRestrictions returns baggage restrictions from collector.
func (s *collectorProxy) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	r, err := s.baggageClient.GetBaggageRestrictions(context.Background(), &api_v2.BaggageRestrictionParameters{
		ServiceName: serviceName,
	})
	if err != nil {
		return nil, err
	}
	return jaeger.ConvertBaggageRestrictionsResponseFromDomain(r)
}
