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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to process gRPC TLS options")
}
