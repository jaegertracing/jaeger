// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage/mocks"
	spanstore_mocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestQueryBuilderFlagsDeprecation(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.port=80",
	})
	qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())
	assert.Equal(t, ":80", qOpts.HostPort)
}

func TestQueryBuilderFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.static-files=/dev/null",
		"--query.ui-config=some.json",
		"--query.base-path=/jaeger",
		"--query.host-port=127.0.0.1:8080",
		"--query.additional-headers=access-control-allow-origin:blerg",
		"--query.additional-headers=whatever:thing",
		"--query.max-clock-skew-adjustment=10s",
	})
	qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())
	assert.Equal(t, "/dev/null", qOpts.StaticAssets)
	assert.Equal(t, "some.json", qOpts.UIConfig)
	assert.Equal(t, "/jaeger", qOpts.BasePath)
	assert.Equal(t, "127.0.0.1:8080", qOpts.HostPort)
	assert.Equal(t, http.Header{
		"Access-Control-Allow-Origin": []string{"blerg"},
		"Whatever":                    []string{"thing"},
	}, qOpts.AdditionalHeaders)
	assert.Equal(t, 10*time.Second, qOpts.MaxClockSkewAdjust)
}

func TestQueryBuilderBadHeadersFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.additional-headers=malformedheader",
	})
	qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())
	assert.Nil(t, qOpts.AdditionalHeaders)
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
	assert.NoError(t, err)

	malformedHeaders := append(headers, "this is not a valid header")
	parsedHeaders, err = stringSliceAsHeader(malformedHeaders)
	assert.Nil(t, parsedHeaders)
	assert.Error(t, err)

	parsedHeaders, err = stringSliceAsHeader([]string{})
	assert.Nil(t, parsedHeaders)
	assert.NoError(t, err)

	parsedHeaders, err = stringSliceAsHeader(nil)
	assert.Nil(t, parsedHeaders)
	assert.NoError(t, err)
}

func TestBuildQueryServiceOptions(t *testing.T) {
	v, _ := config.Viperize(AddFlags)
	qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())
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
	var flagPortCases = []struct {
		name                 string
		flagsArray           []string
		expectedHTTPHostPort string
		expectedGRPCHostPort string
		verifyCommonPort     bool
		expectedHostPort     string
	}{
		{
			// Since TLS is enabled in atleast one server, the dedicated host-ports obtained from viper are used, even if common host-port is specified
			name: "Atleast one dedicated host-port and common host-port is specified, atleast one of GRPC, HTTP TLS enabled",
			flagsArray: []string{
				"--query.grpc.tls.enabled=true",
				"--query.http-server.host-port=127.0.0.1:8081",
				"--query.host-port=127.0.0.1:8080",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), // fallback in viper
			verifyCommonPort:     false,
		},
		{
			// TLS is disabled in both servers, since common host-port is specified, common host-port is used
			name: "Atleast one dedicated host-port is specified, common host-port is specified, both GRPC and HTTP TLS disabled",
			flagsArray: []string{
				"--query.http-server.host-port=127.0.0.1:8081",
				"--query.host-port=127.0.0.1:8080",
			},
			expectedHTTPHostPort: "127.0.0.1:8080",
			expectedGRPCHostPort: "127.0.0.1:8080",
			verifyCommonPort:     true,
			expectedHostPort:     "127.0.0.1:8080",
		},
		{
			// Since TLS is enabled in atleast one server, the dedicated host-ports obtained from viper are used
			name: "Atleast one dedicated host-port is specified, common host-port is not specified, atleast one of GRPC, HTTP TLS enabled",
			flagsArray: []string{
				"--query.grpc.tls.enabled=true",
				"--query.http-server.host-port=127.0.0.1:8081",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), //  fallback in viper
			verifyCommonPort:     false,
		},
		{
			// TLS is disabled in both servers, since common host-port is not specified but atleast one dedicated port is specified, the dedicated host-ports obtained from viper are used
			name: "Atleast one dedicated port, common port defined, both GRPC and HTTP TLS disabled",
			flagsArray: []string{
				"--query.http-server.host-port=127.0.0.1:8081",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), // fallback in viper
			verifyCommonPort:     false,
		},
		{
			// Since TLS is enabled in atleast one server, the dedicated host-ports obtained from viper are used, even if common host-port is specified and the dedicated host-port are not specified
			name: "No dedicated host-port is specified, common host-port is specified, atleast one of GRPC, HTTP TLS enabled",
			flagsArray: []string{
				"--query.grpc.tls.enabled=true",
				"--query.host-port=127.0.0.1:8080",
			},
			expectedHTTPHostPort: ports.PortToHostPort(ports.QueryHTTP), // fallback in viper
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), // fallback in viper
			verifyCommonPort:     false,
		},
		{
			// TLS is disabled in both servers, since only common host-port is specified, common host-port is used
			name: "No dedicated host-port is specified, common host-port is specified, both GRPC and HTTP TLS disabled",
			flagsArray: []string{
				"--query.host-port=127.0.0.1:8080",
			},
			expectedHTTPHostPort: "127.0.0.1:8080",
			expectedGRPCHostPort: "127.0.0.1:8080",
			verifyCommonPort:     true,
			expectedHostPort:     "127.0.0.1:8080",
		},
		{
			// Since TLS is enabled in atleast one server, the dedicated host-ports obtained from viper are used
			name: "No dedicated host-port is specified, common host-port is  not specified, atleast one of GRPC, HTTP TLS enabled",
			flagsArray: []string{
				"--query.grpc.tls.enabled=true",
			},
			expectedHTTPHostPort: ports.PortToHostPort(ports.QueryHTTP), // fallback in viper
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryGRPC), // fallback in viper
			verifyCommonPort:     false,
		},
		{
			// TLS is disabled in both servers, since common host-port is not specified and neither dedicated ports are specified, common host-port from viper is used
			name:                 "No dedicated host-port is specified, common host-port is not specified, both GRPC and HTTP TLS disabled",
			flagsArray:           []string{},
			expectedHTTPHostPort: ports.PortToHostPort(ports.QueryHTTP),
			expectedGRPCHostPort: ports.PortToHostPort(ports.QueryHTTP),
			verifyCommonPort:     true,
			expectedHostPort:     ports.PortToHostPort(ports.QueryHTTP), //  fallback in viper
		},
	}

	for _, test := range flagPortCases {
		t.Run(test.name, func(t *testing.T) {
			v, command := config.Viperize(AddFlags)
			command.ParseFlags(test.flagsArray)
			qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())

			assert.Equal(t, test.expectedHTTPHostPort, qOpts.HTTPHostPort)
			assert.Equal(t, test.expectedGRPCHostPort, qOpts.GRPCHostPort)
			if test.verifyCommonPort {
				assert.Equal(t, test.expectedHostPort, qOpts.HostPort)
			}

		})
	}
}
