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

package clientcfghttp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
)

type mockSamplingProvider struct {
	samplingResponse *api_v2.SamplingStrategyResponse
}

func (m *mockSamplingProvider) GetSamplingStrategy(context.Context, string /* serviceName */) (*api_v2.SamplingStrategyResponse, error) {
	if m.samplingResponse == nil {
		return nil, errors.New("no mock response provided")
	}
	return m.samplingResponse, nil
}

func (*mockSamplingProvider) Close() error {
	return nil
}

type mockBaggageMgr struct {
	baggageResponse []*baggage.BaggageRestriction
}

func (m *mockBaggageMgr) GetBaggageRestrictions(context.Context, string /* serviceName */) ([]*baggage.BaggageRestriction, error) {
	if m.baggageResponse == nil {
		return nil, errors.New("no mock response provided")
	}
	return m.baggageResponse, nil
}

func TestConfigManager(t *testing.T) {
	bgm := &mockBaggageMgr{}
	mgr := &ConfigManager{
		SamplingProvider: &mockSamplingProvider{
			samplingResponse: &api_v2.SamplingStrategyResponse{},
		},
		BaggageManager: bgm,
	}
	t.Run("GetSamplingStrategy", func(t *testing.T) {
		r, err := mgr.GetSamplingStrategy(context.Background(), "foo")
		require.NoError(t, err)
		assert.Equal(t, api_v2.SamplingStrategyResponse{}, *r)
	})
	t.Run("GetBaggageRestrictions", func(t *testing.T) {
		expResp := []*baggage.BaggageRestriction{}
		bgm.baggageResponse = expResp
		r, err := mgr.GetBaggageRestrictions(context.Background(), "foo")
		require.NoError(t, err)
		assert.Equal(t, expResp, r)
	})
	t.Run("GetBaggageRestrictionsError", func(t *testing.T) {
		mgr.BaggageManager = nil
		_, err := mgr.GetBaggageRestrictions(context.Background(), "foo")
		require.EqualError(t, err, "baggage restrictions not implemented")
	})
}
