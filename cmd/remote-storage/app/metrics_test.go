// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

func TestDefaultMetricsConfig(t *testing.T) {
	assert.Equal(t, MetricsConfig{Backend: "prometheus", HTTPRoute: "/metrics"}, DefaultMetricsConfig())
}

func TestMetricsConfigUnmarshal(t *testing.T) {
	b := DefaultMetricsConfig()
	conf := confmap.NewFromStringMap(map[string]any{
		"backend":    "none",
		"http_route": "/m",
	})
	require.NoError(t, conf.Unmarshal(&b))
	assert.Equal(t, "none", b.Backend)
	assert.Equal(t, "/m", b.HTTPRoute)
}

func TestMetricsConfigCreateMetricsFactory(t *testing.T) {
	assertPromCounter := func() {
		families, err := prometheus.DefaultGatherer.Gather()
		require.NoError(t, err)
		for _, mf := range families {
			if mf.GetName() == "foo_counter_total" {
				return
			}
		}
		t.FailNow()
	}
	testCases := []struct {
		backend string
		route   string
		err     error
		handler bool
		assert  func()
	}{
		{
			backend: "prometheus",
			route:   "/",
			handler: true,
			assert:  assertPromCounter,
		},
		{
			backend: "none",
			handler: false,
		},
		{
			backend: "",
			handler: false,
		},
		{
			backend: "invalid",
			err:     errUnknownBackend,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		b := &MetricsConfig{
			Backend:   testCase.backend,
			HTTPRoute: testCase.route,
		}
		mf, err := b.CreateMetricsFactory("foo")
		if testCase.err != nil {
			require.ErrorIs(t, err, testCase.err)
			continue
		}
		require.NotNil(t, mf)
		mf.Counter(metrics.Options{Name: "counter", Tags: nil}).Inc(1)
		if testCase.assert != nil {
			testCase.assert()
		}
		if testCase.handler {
			require.NotNil(t, b.Handler())
		}
	}
}
