// Copyright (c) 2019 The Jaeger Authors.
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
	"context"
	"fmt"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestServerError(t *testing.T) {
	srv := &Server{
		queryOptions: &QueryOptions{
			Port: -1,
		},
	}
	assert.Error(t, srv.Start())
}

func TestServer(t *testing.T) {
	flagsSvc := flags.NewService(ports.AgentAdminHTTP)
	flagsSvc.Logger = zap.NewNop()

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}

	querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})

	tracer := opentracing.NoopTracer{}

	server := NewServer(flagsSvc, querySvc,
		&QueryOptions{Port: ports.QueryAdminHTTP, BearerTokenPropagation: true},
		tracer)
	assert.NoError(t, server.Start())

	client := newGRPCClient(t, fmt.Sprintf(":%d", ports.QueryHTTP))
	defer client.conn.Close()

	var queryErr error
	for i := 0; i < 10; i++ {
		queryErr = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2000*time.Millisecond)
			defer cancel()

			_, err := client.GetTrace(ctx, &api_v2.GetTraceRequest{})
			if err != nil {
				t.Log("cannot GetTrace", err)
			}
			return err
		}()
		if queryErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.NoError(t, queryErr, "Connection test did not succeed")

	// TODO wait for servers to come up and test http and grpc endpoints
	time.Sleep(1 * time.Second)

	server.Close()
	for i := 0; i < 10; i++ {
		if server.svc.HC().Get() == healthcheck.Unavailable {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal(t, healthcheck.Unavailable, server.svc.HC().Get())
}

func TestServerGracefulExit(t *testing.T) {
	flagsSvc := flags.NewService(ports.AgentAdminHTTP)

	zapCore, logs := observer.New(zap.ErrorLevel)
	assert.Equal(t, 0, logs.Len(), "Expected initial ObservedLogs to have zero length.")

	flagsSvc.Logger = zap.New(zapCore)

	querySvc := &querysvc.QueryService{}
	tracer := opentracing.NoopTracer{}
	server := NewServer(flagsSvc, querySvc, &QueryOptions{Port: ports.QueryAdminHTTP}, tracer)
	assert.NoError(t, server.Start())

	// Wait for servers to come up before we can call .Close()
	time.Sleep(1 * time.Second)
	server.Close()

	for _, logEntry := range logs.All() {
		assert.True(t, logEntry.Level != zap.ErrorLevel,
			"Error log found on server exit: %v", logEntry)
	}
}
