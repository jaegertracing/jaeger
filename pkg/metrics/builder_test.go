// Copyright (c) 2017 Uber Technologies, Inc.
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

package metrics

import (
	"expvar"
	"flag"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
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
			if mf.GetName() == "foo_counter" {
				return
			}
		}
		t.FailNow()
	}
	assertExpVarCounter := func() {
		var found expvar.KeyValue
		expected := "foo.counter"
		expvar.Do(func(kv expvar.KeyValue) {
			if kv.Key == expected {
				found = kv
			}
		})
		assert.Equal(t, expected, found.Key)
	}
	testCases := []struct {
		backend  string
		route    string
		tags     string
		err      error
		errViper error
		handler  bool
		assert   func()
	}{
		{
			backend: "expvar",
			route:   "/",
			handler: true,
			assert:  assertExpVarCounter,
		},
		{
			backend: "prometheus",
			route:   "/",
			handler: true,
			assert:  assertPromCounter,
		},
		{
			backend: "influx",
		},
		{
			backend:  "influx",
			tags:     "malformed}json",
			errViper: errMalformedMetricTags,
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
		v, _ := config.Viperize(AddFlags)
		if testCase.tags != "" {
			v.Set("metrics-tags", testCase.tags)
		}

		b := &Builder{}
		_, err := b.InitFromViper(v)
		if testCase.errViper != nil {
			require.Error(t, err, testCase.errViper.Error())
		} else {
			require.Nil(t, err)
		}

		b.Backend = testCase.backend
		b.HTTPRoute = testCase.route

		mf, err := b.CreateMetricsFactory("foo", zap.NewNop())
		if testCase.err != nil {
			assert.Equal(t, err, testCase.err)
			continue
		}
		require.NotNil(t, mf)
		mf.Counter("counter", nil).Inc(1)
		if testCase.assert != nil {
			testCase.assert()
		}
		if testCase.handler {
			require.NotNil(t, b.Handler())
		}
	}
}
