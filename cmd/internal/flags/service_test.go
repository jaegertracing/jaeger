// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"flag"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

func TestAddFlags(*testing.T) {
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
			expErr: "cannot start the admin server: failed to load TLS config",
		},
		{
			name:   "bad host:port",
			flags:  []string{"--admin.http.host-port=invalid"},
			expErr: "cannot start the admin server",
		},
		{
			name:  "clean start",
			flags: []string{},
		},
	}
	for _, test := range scenarios {
		t.Run(test.name, func(t *testing.T) {
			s := NewService( /* default port= */ 0)
			v, cmd := config.Viperize(s.AddFlags)
			err := cmd.ParseFlags(test.flags)
			require.NoError(t, err)
			err = s.Start(v)
			if test.expErr != "" {
				require.ErrorContains(t, err, test.expErr)
				return
			}
			require.NoError(t, err)

			var stopped atomic.Bool
			shutdown := func() {
				stopped.Store(true)
			}
			go s.RunAndThen(shutdown)

			waitForEqual(t, healthcheck.Ready, func() any { return s.HC().Get() })
			s.HC().Set(healthcheck.Unavailable)
			waitForEqual(t, healthcheck.Unavailable, func() any { return s.HC().Get() })

			s.signalsChannel <- os.Interrupt
			waitForEqual(t, true, func() any { return stopped.Load() })
		})
	}
}

func waitForEqual(t *testing.T, expected any, getter func() any) {
	for i := 0; i < 1000; i++ {
		value := getter()
		if reflect.DeepEqual(value, expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, expected, getter())
}
