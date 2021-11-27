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
)

func TestProxyBuilderFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--proxy.static-files=/dev/null",
		"--proxy.ui-config=some.json",
		"--proxy.base-path=/jaeger",
		"--proxy.http-server.host-port=127.0.0.1:8080",
		"--proxy.grpc-server.host-port=127.0.0.1:8081",
		"--proxy.additional-headers=access-control-allow-origin:blerg",
		"--proxy.additional-headers=whatever:thing",
		"--proxy.max-clock-skew-adjustment=10s",
	})
	qOpts := new(ProxyOptions).InitFromViper(v, zap.NewNop())
	assert.Equal(t, "/dev/null", qOpts.StaticAssets)
	assert.Equal(t, "some.json", qOpts.UIConfig)
	assert.Equal(t, "/jaeger", qOpts.BasePath)
	assert.Equal(t, "127.0.0.1:8080", qOpts.HTTPHostPort)
	assert.Equal(t, "127.0.0.1:8081", qOpts.GRPCHostPort)
	assert.Equal(t, http.Header{
		"Access-Control-Allow-Origin": []string{"blerg"},
		"Whatever":                    []string{"thing"},
	}, qOpts.AdditionalHeaders)
	assert.Equal(t, 10*time.Second, qOpts.MaxClockSkewAdjust)
}

func TestQueryBuilderBadHeadersFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--proxy.additional-headers=malformedheader",
	})
	qOpts := new(ProxyOptions).InitFromViper(v, zap.NewNop())
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

func TestBuildProxyServiceOptions(t *testing.T) {
	v, _ := config.Viperize(AddFlags)
	qOpts := new(ProxyOptions).InitFromViper(v, zap.NewNop())
	assert.NotNil(t, qOpts)

	pSvcOpts := qOpts.BuildProxyServiceOptions(zap.NewNop())
	assert.NotNil(t, pSvcOpts)
	assert.NotNil(t, pSvcOpts.Adjuster)

	pSvcOpts = qOpts.BuildProxyServiceOptions(zap.NewNop())
	assert.NotNil(t, pSvcOpts)
	assert.NotNil(t, pSvcOpts.Adjuster)
}

func TestProxyOptionsPortAllocationFromFlags(t *testing.T) {
	var flagPortCases = []struct {
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
			expectedHTTPHostPort: ports.PortToHostPort(ports.ProxyHTTP), // fallback in viper
			expectedGRPCHostPort: ports.PortToHostPort(ports.ProxyGRPC), // fallback in viper
		},
		{
			// If any one host-port is specified, and TLS is diabled, fallback to ports defined in viper
			name: "Atleast one dedicated host-port is specified, both GRPC and HTTP TLS disabled",
			flagsArray: []string{
				"--proxy.http-server.host-port=127.0.0.1:8081",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: ports.PortToHostPort(ports.ProxyGRPC), // fallback in viper
		},
		{
			// Allows usage of common host-ports.  Flags allow this irrespective of TLS status
			// The server with TLS enabled with equal HTTP & GRPC host-ports, is still an acceptable flag configuration, but will throw an error during initialisation
			name: "Common equal host-port specified, TLS enabled in atleast one server",
			flagsArray: []string{
				"--proxy.grpc.tls.enabled=true",
				"--proxy.http-server.host-port=127.0.0.1:8081",
				"--proxy.grpc-server.host-port=127.0.0.1:8081",
			},
			expectedHTTPHostPort: "127.0.0.1:8081",
			expectedGRPCHostPort: "127.0.0.1:8081",
		},
	}

	for _, test := range flagPortCases {
		t.Run(test.name, func(t *testing.T) {
			v, command := config.Viperize(AddFlags)
			command.ParseFlags(test.flagsArray)
			qOpts := new(ProxyOptions).InitFromViper(v, zap.NewNop())

			assert.Equal(t, test.expectedHTTPHostPort, qOpts.HTTPHostPort)
			assert.Equal(t, test.expectedGRPCHostPort, qOpts.GRPCHostPort)

		})
	}
}
