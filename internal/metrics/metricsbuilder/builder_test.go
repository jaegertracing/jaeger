// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metricsbuilder

import (
	"flag"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestAddFlags(t *testing.T) {
	v := viper.New()
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	command.ParseFlags([]string{
		"--metrics-backend=foo",
		"--metrics-http-route=bar",
	})

	b := &Builder{}
	b.InitFromViper(v)

	assert.Equal(t, "foo", b.Backend)
	assert.Equal(t, "bar", b.HTTPRoute)
}

func TestBuilder(t *testing.T) {
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
		b := &Builder{
			Backend:   testCase.backend,
			HTTPRoute: testCase.route,
		}
		mf, err := b.CreateMetricsFactory("foo")
		if testCase.err != nil {
			assert.Equal(t, err, testCase.err)
			continue
		}
		require.NotNil(t, mf)
		mf.Counter(api.Options{Name: "counter", Tags: nil}).Inc(1)
		if testCase.assert != nil {
			testCase.assert()
		}
		if testCase.handler {
			require.NotNil(t, b.Handler())
		}
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
