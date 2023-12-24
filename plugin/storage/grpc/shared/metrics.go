// Copyright (c) 2023 The Jaeger Authors.
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

package shared

import (
	"context"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

var _ metricsstore.Reader = (*metricsReader)(nil)

type metricsReader struct {
	client storage_v1.MetricsReaderPluginClient
}

func (m *metricsReader) GetLatencies(ctx context.Context, params *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	request := &storage_v1.GetLatenciesRequest{
		BaseQueryParameters: baseQueryParametersFromPogo(params.BaseQueryParameters),
		Quantile:            float32(params.Quantile),
	}

	response, err := m.client.GetLatencies(upgradeContext(ctx), request)
	if err != nil {
		return nil, err
	}
	return response.MetricFamily, nil
}

func (m *metricsReader) GetCallRates(ctx context.Context, params *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	request := &storage_v1.GetCallRatesRequest{
		BaseQueryParameters: baseQueryParametersFromPogo(params.BaseQueryParameters),
	}

	response, err := m.client.GetCallRates(upgradeContext(ctx), request)
	if err != nil {
		return nil, err
	}
	return response.MetricFamily, nil
}

func (m *metricsReader) GetErrorRates(ctx context.Context, params *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	request := &storage_v1.GetErrorRatesRequest{
		BaseQueryParameters: baseQueryParametersFromPogo(params.BaseQueryParameters),
	}

	response, err := m.client.GetErrorRates(upgradeContext(ctx), request)
	if err != nil {
		return nil, err
	}
	return response.MetricFamily, nil
}

func (m *metricsReader) GetMinStepDuration(ctx context.Context, params *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	response, err := m.client.GetMinStepDuration(upgradeContext(ctx), &storage_v1.GetMinStepDurationRequest{})
	if err != nil {
		return 0, err
	}

	minStep := time.Second
	if response.MinStep != nil {
		minStep = time.Duration(response.MinStep.Seconds)*time.Second + time.Duration(response.MinStep.Nanos)
	}

	return minStep, nil
}

func baseQueryParametersFromPogo(pogo metricsstore.BaseQueryParameters) *storage_v1.MetricsBaseQueryParameters {
	proto := &storage_v1.MetricsBaseQueryParameters{
		ServiceNames:     pogo.ServiceNames,
		GroupByOperation: pogo.GroupByOperation,
		SpanKinds:        pogo.SpanKinds,
	}

	if pogo.EndTime != nil {
		proto.EndTime = &types.Timestamp{Seconds: pogo.EndTime.Unix(), Nanos: int32(pogo.EndTime.Nanosecond())}
	}
	if pogo.Lookback != nil {
		proto.Lookback = &types.Duration{Seconds: int64(*pogo.Lookback / time.Second), Nanos: int32(*pogo.Lookback % time.Second)}
	}
	if pogo.Step != nil {
		proto.Step = &types.Duration{Seconds: int64(*pogo.Step / time.Second), Nanos: int32(*pogo.Step % time.Second)}
	}
	if pogo.RatePer != nil {
		proto.RatePer = &types.Duration{Seconds: int64(*pogo.RatePer / time.Second), Nanos: int32(*pogo.RatePer % time.Second)}
	}

	return proto
}
