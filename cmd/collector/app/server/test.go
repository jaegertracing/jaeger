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
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type mockSamplingStore struct{}

func (s mockSamplingStore) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, nil
}

type mockSpanProcessor struct {
}

func (p *mockSpanProcessor) ProcessSpans(spans []*model.Span, _ processor.SpansOptions) ([]bool, error) {
	return []bool{}, nil
}
