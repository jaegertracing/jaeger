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
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	testCases := []struct {
		backend string
		route   string
		err     error
		handler bool
	}{
		{
			backend: "expvar",
			route:   "/",
			handler: true,
		},
		{
			backend: "prometheus",
			route:   "/",
			handler: true,
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
		if testCase.handler {
			require.NotNil(t, b.Handler())
		}
	}
}
