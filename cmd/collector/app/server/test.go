// Copyright (c) 2020 The Jaeger Authors.
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

package server

import (
	"context"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

type mockSamplingStore struct{}

func (s mockSamplingStore) GetSamplingStrategy(_ context.Context, serviceName string) (*api_v2.SamplingStrategyResponse, error) {
	return nil, nil
}

func (s mockSamplingStore) Close() error {
	return nil
}

type mockSpanProcessor struct{}

func (p *mockSpanProcessor) Close() error {
	return nil
}

func (p *mockSpanProcessor) ProcessSpans(spans []*model.Span, _ processor.SpansOptions) ([]bool, error) {
	return []bool{}, nil
}
