// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func TestBuildRemoteNewClientError(t *testing.T) {
	// this is a silly test to verify handling of error from grpc.NewClient, which cannot be induced via params.
	c := &Configuration{}
	_, err := c.buildRemote(zap.NewNop(), nil, func(target string, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		return nil, errors.New("test error")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating remote storage client")
}
