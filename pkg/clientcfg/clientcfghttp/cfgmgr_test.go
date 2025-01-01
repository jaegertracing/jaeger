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

func TestConfigManager(t *testing.T) {
	mgr := &ConfigManager{
		SamplingProvider: &mockSamplingProvider{
			samplingResponse: &api_v2.SamplingStrategyResponse{},
		},
	}
	t.Run("GetSamplingStrategy", func(t *testing.T) {
		r, err := mgr.GetSamplingStrategy(context.Background(), "foo")
		require.NoError(t, err)
		assert.Equal(t, api_v2.SamplingStrategyResponse{}, *r)
	})
}
