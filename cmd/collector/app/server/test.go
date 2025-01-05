// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
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

func (*mockSpanProcessor) ProcessSpans(_ processor.Spans) ([]bool, error) {
	return []bool{}, nil
}
