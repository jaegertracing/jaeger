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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type mockSamplingStore struct {
	samplingResponse *sampling.SamplingStrategyResponse
}

func (m *mockSamplingStore) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	if m.samplingResponse == nil {
		return nil, errors.New("no mock response provided")
	}
	return m.samplingResponse, nil
}

type mockBaggageMgr struct {
	baggageResponse []*baggage.BaggageRestriction
}

func (m *mockBaggageMgr) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	if m.baggageResponse == nil {
		return nil, errors.New("no mock response provided")
	}
	return m.baggageResponse, nil
}

func TestConfigManager(t *testing.T) {
	bgm := &mockBaggageMgr{}
	mgr := &ConfigManager{
		SamplingStrategyStore: &mockSamplingStore{
			samplingResponse: &sampling.SamplingStrategyResponse{},
		},
		BaggageManager: bgm,
	}
	t.Run("GetSamplingStrategy", func(t *testing.T) {
		r, err := mgr.GetSamplingStrategy("foo")
		require.NoError(t, err)
		assert.Equal(t, sampling.SamplingStrategyResponse{}, *r)
	})
	t.Run("GetBaggageRestrictions", func(t *testing.T) {
		expResp := []*baggage.BaggageRestriction{}
		bgm.baggageResponse = expResp
		r, err := mgr.GetBaggageRestrictions("foo")
		require.NoError(t, err)
		assert.Equal(t, expResp, r)
	})
	t.Run("GetBaggageRestrictionsError", func(t *testing.T) {
		mgr.BaggageManager = nil
		_, err := mgr.GetBaggageRestrictions("foo")
		assert.EqualError(t, err, "baggage not implemented")
	})
}
