// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
)

type mockSamplingProvider struct{}

func (mockSamplingProvider) GetSamplingStrategy(context.Context, string /* serviceName */) (*api_v2.SamplingStrategyResponse, error) {
	return nil, nil
}

func (mockSamplingProvider) Close() error {
	return nil
}

type mockSpanProcessor struct{}

func (*mockSpanProcessor) Close() error {
	return nil
}

func (*mockSpanProcessor) ProcessSpans(_ context.Context, _ processor.Batch) ([]bool, error) {
	return []bool{}, nil
}
