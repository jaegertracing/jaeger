// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package promcfg

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestValidate(t *testing.T) {
	cfg := Configuration{
		ServerURL: "localhost:1234",
	}
	err := cfg.Validate()
	require.NoError(t, err)
	cfg = Configuration{}
	err = cfg.Validate()
	require.Error(t, err)
}

func TestValidateLatencyUnit(t *testing.T) {
	tests := []struct {
		name        string
		latencyUnit string
		wantErr     string
	}{
		{name: "empty is allowed (default applied later)", latencyUnit: ""},
		{name: "ms is valid", latencyUnit: "ms"},
		{name: "s is valid", latencyUnit: "s"},
		{name: "invalid unit is rejected", latencyUnit: "us", wantErr: "latency_unit"},
		{name: "long form is rejected", latencyUnit: "milliseconds", wantErr: "latency_unit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Configuration{
				ServerURL:   "localhost:1234",
				LatencyUnit: test.latencyUnit,
			}
			err := cfg.Validate()
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
