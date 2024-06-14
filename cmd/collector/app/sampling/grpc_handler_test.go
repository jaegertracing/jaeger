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

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

type mockSamplingStore struct{}

func (mockSamplingStore) GetSamplingStrategy(_ context.Context, serviceName string) (*api_v2.SamplingStrategyResponse, error) {
	if serviceName == "error" {
		return nil, errors.New("some error")
	} else if serviceName == "nil" {
		return nil, nil
	}
	return &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC}, nil
}

func (mockSamplingStore) Close() error {
	return nil
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
			require.EqualError(t, err, test.err)
			require.Nil(t, resp)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.resp, resp)
		}
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
