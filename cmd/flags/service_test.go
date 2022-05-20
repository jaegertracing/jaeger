// Copyright (c) 2022 The Jaeger Authors.
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
package flags

import (
	"flag"
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"github.com/crossdock/crossdock-go/require"
	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestAddFlags(t *testing.T) {
	s := NewService(0)
	s.AddFlags(new(flag.FlagSet))

	s.NoStorage = true
	s.AddFlags(new(flag.FlagSet))
}

func TestStartErrors(t *testing.T) {
	scenarios := []struct {
		name   string
		flags  []string
		expErr string
	}{
		{
			name:   "bad config",
			flags:  []string{"--config-file=invalid-file-name"},
			expErr: "cannot load config file",
		},
		{
			name:   "bad log level",
			flags:  []string{"--log-level=invalid-log-level"},
			expErr: "cannot create logger",
		},
		{
			name:   "bad metrics backend",
			flags:  []string{"--metrics-backend=invalid-metrics-backend"},
			expErr: "cannot create metrics factory",
		},
		{
			name:   "bad admin TLS",
			flags:  []string{"--admin.http.tls.enabled=true", "--admin.http.tls.cert=invalid-cert"},
			expErr: "cannot initialize admin server",
		},
		{
			name:   "bad host:port",
			flags:  []string{"--admin.http.host-port=invalid"},
			expErr: "cannot start the admin server",
		},
	}
	for _, test := range scenarios {
		t.Run(test.name, func(t *testing.T) {
			s := NewService( /*default port=*/ 0)
			v, cmd := config.Viperize(s.AddFlags)
			err := cmd.ParseFlags(test.flags)
			assert.NoError(t, err)
			err = s.Start(v)
			if test.expErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
