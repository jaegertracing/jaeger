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

func TestQueryBuilderFlagsSeparatePorts(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--query.http-server.host-port=127.0.0.1:8080",
	})
	qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())
	assert.Equal(t, "127.0.0.1:8080", qOpts.HTTPHostPort)
	assert.Equal(t, ports.PortToHostPort(ports.QueryGRPC), qOpts.GRPCHostPort)
}

func TestQueryBuilderFlagsSeparateNoPorts(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{})
	qOpts := new(QueryOptions).InitFromViper(v, zap.NewNop())

	assert.Equal(t, ports.PortToHostPort(ports.QueryHTTP), qOpts.HTTPHostPort)
	assert.Equal(t, ports.PortToHostPort(ports.QueryHTTP), qOpts.GRPCHostPort)
	assert.Equal(t, ports.PortToHostPort(ports.QueryHTTP), qOpts.HostPort)
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
