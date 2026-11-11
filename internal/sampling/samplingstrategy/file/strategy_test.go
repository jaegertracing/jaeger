// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
)

func TestStrategiesValidate(t *testing.T) {
	tests := []struct {
		name       string
		strategies *strategies
		wantErr    string
	}{
		{
			name:       "nil strategies",
			strategies: nil,
		},
		{
			name:       "empty strategies",
			strategies: &strategies{},
		},
		{
			name: "valid params",
			strategies: &strategies{
				DefaultStrategy: &serviceStrategy{
					strategy: strategy{Type: samplerTypeProbabilistic, Param: 0.5},
					OperationStrategies: []*operationStrategy{
						{Operation: "op", strategy: strategy{Type: samplerTypeProbabilistic, Param: 1}},
					},
				},
				ServiceStrategies: []*serviceStrategy{
					{Service: "a", strategy: strategy{Type: samplerTypeProbabilistic, Param: 0}},
					{Service: "b", strategy: strategy{Type: samplerTypeRateLimiting, Param: 0}},
					{Service: "c", strategy: strategy{Type: samplerTypeRateLimiting, Param: math.MaxInt32}},
				},
			},
		},
		{
			name: "unknown type is left to parseStrategy",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "a", strategy: strategy{Type: "blah", Param: 42}},
				},
			},
		},
		{
			name: "default probabilistic above 1",
			strategies: &strategies{
				DefaultStrategy: &serviceStrategy{strategy: strategy{Type: samplerTypeProbabilistic, Param: 1.5}},
			},
			wantErr: "default strategy: probabilistic sampling param must be between 0 and 1, got 1.5",
		},
		{
			name: "service probabilistic negative",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "svc", strategy: strategy{Type: samplerTypeProbabilistic, Param: -0.2}},
				},
			},
			wantErr: `service "svc": probabilistic sampling param must be between 0 and 1, got -0.2`,
		},
		{
			name: "service probabilistic NaN",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "svc", strategy: strategy{Type: samplerTypeProbabilistic, Param: math.NaN()}},
				},
			},
			wantErr: `service "svc": probabilistic sampling param must be between 0 and 1, got NaN`,
		},
		{
			name: "service ratelimiting negative",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "svc", strategy: strategy{Type: samplerTypeRateLimiting, Param: -5}},
				},
			},
			wantErr: `service "svc": ratelimiting sampling param must be between 0 and 2147483647, got -5`,
		},
		{
			name: "service ratelimiting above int32",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "svc", strategy: strategy{Type: samplerTypeRateLimiting, Param: 3e9}},
				},
			},
			wantErr: `service "svc": ratelimiting sampling param must be between 0 and 2147483647, got 3e+09`,
		},
		{
			name: "service ratelimiting fractional is truncated by parseStrategy, not rejected",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "svc", strategy: strategy{Type: samplerTypeRateLimiting, Param: 0.5}},
				},
			},
		},
		{
			name: "operation probabilistic above 1",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{
						Service:  "svc",
						strategy: strategy{Type: samplerTypeProbabilistic, Param: 0.5},
						OperationStrategies: []*operationStrategy{
							{Operation: "op", strategy: strategy{Type: samplerTypeProbabilistic, Param: 10}},
						},
					},
				},
			},
			wantErr: `service "svc", operation "op": probabilistic sampling param must be between 0 and 1, got 10`,
		},
		{
			name: "null service strategy",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{Service: "a", strategy: strategy{Type: samplerTypeProbabilistic, Param: 0.5}},
					nil,
				},
			},
			wantErr: "service strategy 1 must not be null",
		},
		{
			name: "null operation strategy",
			strategies: &strategies{
				ServiceStrategies: []*serviceStrategy{
					{
						Service:             "svc",
						strategy:            strategy{Type: samplerTypeProbabilistic, Param: 0.5},
						OperationStrategies: []*operationStrategy{nil},
					},
				},
			},
			wantErr: `service "svc": operation strategy 0 must not be null`,
		},
		{
			name: "null default operation strategy",
			strategies: &strategies{
				DefaultStrategy: &serviceStrategy{
					strategy:            strategy{Type: samplerTypeProbabilistic, Param: 0.5},
					OperationStrategies: []*operationStrategy{nil},
				},
			},
			wantErr: "default strategy: operation strategy 0 must not be null",
		},
		{
			name: "default operation ratelimiting negative",
			strategies: &strategies{
				DefaultStrategy: &serviceStrategy{
					strategy: strategy{Type: samplerTypeProbabilistic, Param: 0.5},
					OperationStrategies: []*operationStrategy{
						{Operation: "op", strategy: strategy{Type: samplerTypeRateLimiting, Param: -1}},
					},
				},
			},
			wantErr: `default strategy, operation "op": ratelimiting sampling param must be between 0 and 2147483647, got -1`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.strategies.validate()
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func writeStrategiesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "strategies.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNewProviderRejectsInvalidParam(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "probabilistic above 1",
			content: `{"service_strategies":[{"service":"svc","type":"probabilistic","param":1.5}]}`,
			wantErr: `invalid sampling strategies: service "svc": probabilistic sampling param must be between 0 and 1, got 1.5`,
		},
		{
			name:    "ratelimiting negative",
			content: `{"service_strategies":[{"service":"svc","type":"ratelimiting","param":-5}]}`,
			wantErr: `invalid sampling strategies: service "svc": ratelimiting sampling param must be between 0 and 2147483647, got -5`,
		},
		{
			name:    "ratelimiting above int32",
			content: `{"service_strategies":[{"service":"svc","type":"ratelimiting","param":3000000000}]}`,
			wantErr: `invalid sampling strategies: service "svc": ratelimiting sampling param must be between 0 and 2147483647, got 3e+09`,
		},
		{
			name:    "null service strategy",
			content: `{"default_strategy":{"type":"probabilistic","param":0.5},"service_strategies":[null]}`,
			wantErr: "invalid sampling strategies: service strategy 0 must not be null",
		},
		{
			name:    "null operation strategy",
			content: `{"service_strategies":[{"service":"svc","type":"probabilistic","param":0.5,"operation_strategies":[null]}]}`,
			wantErr: `invalid sampling strategies: service "svc": operation strategy 0 must not be null`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeStrategiesFile(t, test.content)
			_, err := NewProvider(Options{StrategiesFile: path}, zap.NewNop())
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestReloadKeepsLastStrategiesOnInvalidParam(t *testing.T) {
	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	s, err := NewProvider(Options{
		StrategiesFile: "fixtures/strategies.json",
		ReloadInterval: time.Hour,
	}, logger)
	require.NoError(t, err)
	provider := s.(*samplingProvider)
	defer provider.Close()

	before, err := provider.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	require.Equal(t, api_v2.SamplingStrategyType_PROBABILISTIC, before.StrategyType)
	require.InDelta(t, 0.8, before.ProbabilisticSampling.SamplingRate, 0.01)

	invalid := writeStrategiesFile(t, `{"service_strategies":[{"service":"foo","type":"probabilistic","param":10}]}`)
	assert.Equal(t, "last-good", provider.reloadSamplingStrategy(provider.samplingStrategyLoader(invalid), "last-good"))
	entries := logs.FilterMessage("failed to update sampling strategies").All()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].ContextMap()["error"], `service "foo": probabilistic sampling param must be between 0 and 1, got 10`)

	after, err := provider.GetSamplingStrategy(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
