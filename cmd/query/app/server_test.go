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
	"github.com/stretchr/testify/mock"
	"net/http"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"

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

	server := NewServer(flagsSvc, querySvc, &QueryOptions{Port: ports.QueryAdminHTTP,
		BearerTokenPropagation: true}, tracer)
	assert.NoError(t, server.Start())

	// wait for the server to come up
	time.Sleep(1 * time.Second)

	expectedServices := []string{"demo"}

	// test gRPC endpoint
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", ports.QueryAdminHTTP), grpc.WithInsecure(), grpc.WithTimeout((1 * time.Second)))
	if err != nil {
		t.Errorf("cannot connect to gRPC query service: %v", err)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

		client := api_v2.NewQueryServiceClient(conn)
		resp, err := client.GetServices(ctx, &api_v2.GetServicesRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, resp.Services, expectedServices)

		cancel()

		assert.NoError(t, conn.Close())
	}

	// test http endpoint
	spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/api/services", ports.QueryAdminHTTP), nil)
	assert.NoError(t, err)
	req.Header.Add("Accept", "application/json")
	httpClient = &http.Client{ Timeout: 2 * time.Second }
	resp, err := httpClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

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

	closed := make(chan struct{})
	go func() {
		server.Close()
		close(closed)
	}()
	select {
	case <-closed:
		// all is well
	case <-time.After(1 * time.Second):
		t.Errorf("timeout while stopping server")
	}

	for _, logEntry := range logs.All() {
		assert.True(t, logEntry.Level != zap.ErrorLevel,
			fmt.Sprintf("Error log found on server exit: %v", logEntry))
	}
}
