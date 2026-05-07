// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"testing"

	"go.uber.org/zap"
)

func FuzzUpdateSamplingStrategy(f *testing.F) {
	// Add some valid seeds from fixtures
	f.Add([]byte(`{"default_strategy":{"type":"probabilistic","param":0.5}}`))
	f.Add([]byte(`{"service_strategies":[{"service":"foo","type":"ratelimiting","param":10}]}`))
	f.Add([]byte(`{"default_strategy":{"type":"probabilistic","param":0.5},"service_strategies":[{"service":"foo","operation_strategies":[{"operation":"bar","type":"probabilistic","param":0.8}]}]}`))

	h := &samplingProvider{
		logger:  zap.NewNop(),
		options: Options{DefaultSamplingProbability: 0.001},
	}
	h.storedStrategies.Store(defaultStrategies(0.001))

	f.Fuzz(func(t *testing.T, data []byte) {
		// We don't care about the error, just that it doesn't panic
		_ = h.updateSamplingStrategy(data)
	})
}
