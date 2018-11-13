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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type mockSamplingStore struct{}

func (s mockSamplingStore) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	if serviceName == "error" {
		return nil, errors.New("some error")
	} else if serviceName == "nil" {
		return nil, nil
	}
	return &sampling.SamplingStrategyResponse{StrategyType: sampling.SamplingStrategyType_PROBABILISTIC}, nil
}

func TestNewGRPCHandler(t *testing.T) {
	tests := []struct {
		req  *api_v2.SamplingStrategyParameters
		resp *api_v2.SamplingStrategyResponse
		err  string
	}{
		{req: &api_v2.SamplingStrategyParameters{ServiceName: "error"}, err: "some error"},
		{req: &api_v2.SamplingStrategyParameters{ServiceName: "nil"}, resp: nil},
		{req: &api_v2.SamplingStrategyParameters{ServiceName: "foo"}, resp: &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC}},
	}
	h := NewGRPCHandler(mockSamplingStore{})
	for _, test := range tests {
		resp, err := h.GetSamplingStrategy(context.Background(), test.req)
		if test.err != "" {
			assert.EqualError(t, err, test.err)
			require.Nil(t, resp)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.resp, resp)
		}
	}
}
