// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildRemoteNewClientError(t *testing.T) {
	// this is a silly test to verify handling of error from grpc.NewClient, which cannot be induced via params.
	c := &ConfigV2{}
	_, err := c.buildRemote(zap.NewNop(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating remote storage client")
}
