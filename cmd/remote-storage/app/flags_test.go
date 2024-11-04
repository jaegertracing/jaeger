// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--grpc.host-port=127.0.0.1:8081",
	})
	qOpts, err := new(Options).InitFromViper(v, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8081", qOpts.GRPCHostPort)
}

func TestFailedTLSFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	err := command.ParseFlags([]string{
		"--grpc.tls.enabled=false",
		"--grpc.tls.cert=blah", // invalid unless tls.enabled
	})
	require.NoError(t, err)
	_, err = new(Options).InitFromViper(v, zap.NewNop())
	assert.ErrorContains(t, err, "failed to process gRPC TLS options")
}
