// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
