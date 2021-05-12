// Copyright (c) 2021 The Jaeger Authors.
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

package metricsstore

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

func TestNewMetricsReaderSingleListenedHostPort(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	logger := zap.NewNop()
	addr := listener.Addr().String()

	reader, err := NewMetricsReader(logger, []string{addr}, time.Second)

	assert.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestNewMetricsReaderOneOfTwoListeningOnHostPort(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	logger := zap.NewNop()
	addr := listener.Addr().String()

	reader, err := NewMetricsReader(logger, []string{"nooneislistening:1234", addr}, time.Second)

	assert.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestNewMetricsReaderMultipleErrors(t *testing.T) {
	logger := zap.NewNop()

	reader, err := NewMetricsReader(logger, []string{"localhost:12345", "localhost:12346"}, time.Nanosecond)

	const wantErrMsg = "none of the provided prometheus query host:ports are reachable: [dial tcp: i/o timeout, dial tcp: i/o timeout]"
	assert.EqualError(t, err, wantErrMsg)
	assert.Nil(t, reader)
}

func TestNewMetricsReaderInvalidHostPort(t *testing.T) {
	logger := zap.NewNop()

	reader, err := NewMetricsReader(logger, []string{"invalidhostport"}, time.Second)

	const wantErrMsg = "none of the provided prometheus query host:ports are reachable: address invalidhostport: missing port in address"
	assert.EqualError(t, err, wantErrMsg)
	assert.Nil(t, reader)
}

func TestNewMetricsReaderMissingHostPort(t *testing.T) {
	logger := zap.NewNop()
	var emptyHostPorts []string

	reader, err := NewMetricsReader(logger, emptyHostPorts, time.Second)

	const wantErrMsg = "no prometheus query host:port provided"
	assert.EqualError(t, err, wantErrMsg)
	assert.Nil(t, reader)
}

func TestGetMinStepDuration(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	addr := listener.Addr().String()
	params := metricsstore.MinStepDurationQueryParameters{}
	logger := zap.NewNop()

	reader, err := NewMetricsReader(logger, []string{addr}, time.Second)
	assert.NoError(t, err)

	minStep, err := reader.GetMinStepDuration(context.Background(), &params)
	assert.NoError(t, err)
	assert.Equal(t, time.Millisecond, minStep)
}

func TestGetLatencies(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	addr := listener.Addr().String()
	params := metricsstore.LatenciesQueryParameters{}
	logger := zap.NewNop()

	reader, err := NewMetricsReader(logger, []string{addr}, time.Second)
	assert.NoError(t, err)

	m, err := reader.GetLatencies(context.Background(), &params)
	assert.NoError(t, err)
	assert.Nil(t, m)
}

func TestGetCallRates(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	addr := listener.Addr().String()
	params := metricsstore.CallRateQueryParameters{}
	logger := zap.NewNop()

	reader, err := NewMetricsReader(logger, []string{addr}, time.Second)
	assert.NoError(t, err)

	m, err := reader.GetCallRates(context.Background(), &params)
	assert.NoError(t, err)
	assert.Nil(t, m)
}

func TestGetErrorRates(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	addr := listener.Addr().String()
	params := metricsstore.ErrorRateQueryParameters{}
	logger := zap.NewNop()

	reader, err := NewMetricsReader(logger, []string{addr}, time.Second)
	assert.NoError(t, err)

	m, err := reader.GetErrorRates(context.Background(), &params)
	assert.NoError(t, err)
	assert.Nil(t, m)
}
