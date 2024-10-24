// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage/mocks"
	spanstore_mocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestQueryBuilderFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.static-files=/dev/null",
		"--query.log-static-assets-access=true",
		"--query.ui-config=some.json",
		"--query.base-path=/jaeger",
		"--query.http-server.host-port=127.0.0.1:8080",
		"--query.grpc-server.host-port=127.0.0.1:8081",
		"--query.additional-headers=access-control-allow-origin:blerg",
		"--query.additional-headers=whatever:thing",
		"--query.max-clock-skew-adjustment=10s",
	})
	qOpts, err := new(QueryOptions).InitFromViper(v, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "/dev/null", qOpts.UIConfig.AssetsPath)
	assert.True(t, qOpts.UIConfig.LogAccess)
	assert.Equal(t, "some.json", qOpts.UIConfig.ConfigFile)
	assert.Equal(t, "/jaeger", qOpts.BasePath)
	assert.Equal(t, "127.0.0.1:8080", qOpts.HTTP.Endpoint)
	assert.Equal(t, "127.0.0.1:8081", qOpts.GRPC.NetAddr.Endpoint)
	assert.Equal(t, map[string]configopaque.String{
		"Access-Control-Allow-Origin": "blerg",
		"Whatever":                    "thing",
	}, qOpts.HTTP.ResponseHeaders)
	assert.Equal(t, 10*time.Second, qOpts.MaxClockSkewAdjust)
}

func TestQueryBuilderBadHeadersFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.additional-headers=malformedheader",
	})
	qOpts, err := new(QueryOptions).InitFromViper(v, zap.NewNop())
	require.NoError(t, err)
	assert.Nil(t, qOpts.HTTP.ResponseHeaders)
}

func TestStringSliceAsHeader(t *testing.T) {
	headers := []string{
		"Access-Control-Allow-Origin: https://mozilla.org",
		"Access-Control-Expose-Headers: X-My-Custom-Header",
		"Access-Control-Expose-Headers: X-Another-Custom-Header",
	}

	parsedHeaders, err := stringSliceAsHeader(headers)

	assert.Equal(t, []string{"https://mozilla.org"}, parsedHeaders["Access-Control-Allow-Origin"])
	assert.Equal(t, []string{"X-My-Custom-Header", "X-Another-Custom-Header"}, parsedHeaders["Access-Control-Expose-Headers"])
	require.NoError(t, err)

	malformedHeaders := append(headers, "this is not a valid header")
	parsedHeaders, err = stringSliceAsHeader(malformedHeaders)
	assert.Nil(t, parsedHeaders)
	require.Error(t, err)

	parsedHeaders, err = stringSliceAsHeader([]string{})
	assert.Nil(t, parsedHeaders)
	require.NoError(t, err)

	parsedHeaders, err = stringSliceAsHeader(nil)
	assert.Nil(t, parsedHeaders)
	require.NoError(t, err)
}

func TestBuildQueryServiceOptions(t *testing.T) {
	v, _ := config.Viperize(AddFlags)
	qOpts, err := new(QueryOptions).InitFromViper(v, zap.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, qOpts)

	qSvcOpts := qOpts.BuildQueryServiceOptions(&mocks.Factory{}, zap.NewNop())
	assert.NotNil(t, qSvcOpts)
	assert.NotNil(t, qSvcOpts.Adjuster)
	assert.Nil(t, qSvcOpts.ArchiveSpanReader)
	assert.Nil(t, qSvcOpts.ArchiveSpanWriter)

	comboFactory := struct {
		*mocks.Factory
		*mocks.ArchiveFactory
	}{
		&mocks.Factory{},
		&mocks.ArchiveFactory{},
	}

	comboFactory.ArchiveFactory.On("CreateArchiveSpanReader").Return(&spanstore_mocks.Reader{}, nil)
	comboFactory.ArchiveFactory.On("CreateArchiveSpanWriter").Return(&spanstore_mocks.Writer{}, nil)

	qSvcOpts = qOpts.BuildQueryServiceOptions(comboFactory, zap.NewNop())
	assert.NotNil(t, qSvcOpts)
	assert.NotNil(t, qSvcOpts.Adjuster)
	assert.NotNil(t, qSvcOpts.ArchiveSpanReader)
	assert.NotNil(t, qSvcOpts.ArchiveSpanWriter)
}

func TestQueryOptionsPortAllocationFromFlags(t *testing.T) {
	flagPortCases := []struct {
		name                 string
		flagsArray           []string
		expectedHTTPHostPort string
		expectedGRPCHostPort string
		verifyCommonPort     bool
		expectedHostPort     string
	}{
		{
			// Default behavior. Dedicated host-port is used for both HTTP and GRPC endpoints
			name:                 "No host-port flags specified, both GRPC and HTTP TLS disabled",
			flagsArray:           []string{},
			expectedHTTPHostPort: ports.PortToHostPort(ports.QueryHTTP), // fallback in viper
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), // fallback in viper
		},
		{
			// If any one host-port is specified, and TLS is disabled, fallback to ports defined in viper
			name: "Atleast one dedicated host-port is specified, both GRPC and HTTP TLS disabled",
			flagsArray: []string{
				"--query.http-server.host-port=127.0.0.1:8081",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), // fallback in viper
		},
		{
			// Allows usage of common host-ports.  Flags allow this irrespective of TLS status
			// The server with TLS enabled with equal HTTP & GRPC host-ports, is still an acceptable flag configuration, but will throw an error during initialisation
			name: "Common equal host-port specified, TLS enabled in atleast one server",
			flagsArray: []string{
				"--query.grpc.tls.enabled=true",
				"--query.http-server.host-port=127.0.0.1:8081",
				"--query.grpc-server.host-port=127.0.0.1:8081",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: "127.0.0.1:8081",
		},
	}

	for _, test := range flagPortCases {
		t.Run(test.name, func(t *testing.T) {
			v, command := config.Viperize(AddFlags)
			command.ParseFlags(test.flagsArray)
			qOpts, err := new(QueryOptions).InitFromViper(v, zap.NewNop())
			require.NoError(t, err)

			assert.Equal(t, test.expectedHTTPHostPort, qOpts.HTTP.Endpoint)
			assert.Equal(t, test.expectedGRPCHostPort, qOpts.GRPC.NetAddr.Endpoint)
		})
	}
}

func TestQueryOptions_FailedTLSFlags(t *testing.T) {
	for _, test := range []string{"gRPC", "HTTP"} {
		t.Run(test, func(t *testing.T) {
			proto := strings.ToLower(test)
			v, command := config.Viperize(AddFlags)
			err := command.ParseFlags([]string{
				"--query." + proto + ".tls.enabled=false",
				"--query." + proto + ".tls.cert=blah", // invalid unless tls.enabled
			})
			require.NoError(t, err)
			_, err = new(QueryOptions).InitFromViper(v, zap.NewNop())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to process "+test+" TLS options")
		})
	}
}

func TestQueryOptions_SamePortsLogsWarning(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.http-server.host-port=127.0.0.1:8081",
		"--query.grpc-server.host-port=127.0.0.1:8081",
	})
	_, err := new(QueryOptions).InitFromViper(v, logger)
	require.NoError(t, err)

	require.Contains(
		t,
		logBuf.String(),
		"using the same port for gRPC and HTTP is deprecated",
	)
}

func TestDefaultQueryOptions(t *testing.T) {
	qo := DefaultQueryOptions()
	require.Equal(t, ":16686", qo.HTTP.Endpoint)
	require.Equal(t, ":16685", qo.GRPC.NetAddr.Endpoint)
	require.EqualValues(t, "tcp", qo.GRPC.NetAddr.Transport)
}
