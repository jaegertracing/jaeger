// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
