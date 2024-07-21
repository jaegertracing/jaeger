// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"google.golang.org/grpc"
)

func TestBuildRemoteNewClientError(t *testing.T) {
	// this is a silly test to verify handling of error from grpc.NewClient, which cannot be induced via params.
	c := &ConfigV2{}
	newClientFn := func(_ ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		return nil, errors.New("test error")
	}
	_, err := newRemoteStorage(c, component.TelemetrySettings{}, newClientFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating remote storage client")
}

func TestDefaultConfigV2(t *testing.T) {
	cfg := DefaultConfigV2()
	assert.NotEmpty(t, cfg.Timeout)
}
