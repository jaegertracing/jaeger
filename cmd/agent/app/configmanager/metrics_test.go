// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

type noopManager struct{}

func (noopManager) GetSamplingStrategy(_ context.Context, s string) (*api_v2.SamplingStrategyResponse, error) {
	if s == "failed" {
		return nil, errors.New("failed")
	}
	return &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC}, nil
}

func TestMetrics(t *testing.T) {
	tests := []struct {
		expected []metricstest.ExpectedMetric
		err      error
	}{
		{expected: []metricstest.ExpectedMetric{
			{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "sampling"}, Value: 1},
			{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "sampling"}, Value: 0},
		}},
		{expected: []metricstest.ExpectedMetric{
			{Name: "collector-proxy", Tags: map[string]string{"result": "ok", "endpoint": "sampling"}, Value: 0},
			{Name: "collector-proxy", Tags: map[string]string{"result": "err", "endpoint": "sampling"}, Value: 1},
		}, err: errors.New("failed")},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			metricsFactory := metricstest.NewFactory(time.Microsecond)
			defer metricsFactory.Stop()
			mgr := WrapWithMetrics(&noopManager{}, metricsFactory)

			if test.err != nil {
				s, err := mgr.GetSamplingStrategy(context.Background(), test.err.Error())
				require.Nil(t, s)
				require.EqualError(t, err, test.err.Error())
			} else {
				s, err := mgr.GetSamplingStrategy(context.Background(), "")
				require.NoError(t, err)
				require.NotNil(t, s)
			}
			metricsFactory.AssertCounterMetrics(t, test.expected...)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
