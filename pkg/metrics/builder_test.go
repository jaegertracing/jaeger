// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package metrics

import (
	"flag"
	"net/http"
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
			require.NotNil(t, b.handler)
			mux := http.NewServeMux()
			b.RegisterHandler(mux)
		}
	}
}
