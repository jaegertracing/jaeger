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

package apiv3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var testCertKeyLocation = "../../../../pkg/config/tlscfg/testdata/"

func testGRPCGateway(t *testing.T, basePath string, serverTLS tlscfg.Options, clientTLS tlscfg.Options) {
	testGRPCGatewayWithTenancy(t, basePath, serverTLS, clientTLS,
		tenancy.Options{
			Enabled: false,
		},
		func(*http.Request) {})
}

func setupGRPCGateway(t *testing.T, basePath string, serverTLS tlscfg.Options, clientTLS tlscfg.Options, tenancyOptions tenancy.Options) (*spanstoremocks.Reader, net.Listener, *grpc.Server, context.CancelFunc, *http.Server) {
	r := &spanstoremocks.Reader{}

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})

	var serverGRPCOpts []grpc.ServerOption
	if serverTLS.Enabled {
		config, err := serverTLS.Config(zap.NewNop())
		require.NoError(t, err)
		creds := credentials.NewTLS(config)
		serverGRPCOpts = append(serverGRPCOpts, grpc.Creds(creds))
	}
	if tenancyOptions.Enabled {
		tm := tenancy.NewManager(&tenancyOptions)
		serverGRPCOpts = append(serverGRPCOpts,
			grpc.StreamInterceptor(tenancy.NewGuardingStreamInterceptor(tm)),
			grpc.UnaryInterceptor(tenancy.NewGuardingUnaryInterceptor(tm)),
		)
	}
	grpcServer := grpc.NewServer(serverGRPCOpts...)
	h := &Handler{
		QueryService: q,
	}
	api_v3.RegisterQueryServiceServer(grpcServer, h)
	lis, _ := net.Listen("tcp", ":0")
	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()

	router := &mux.Router{}
	router = router.PathPrefix(basePath).Subrouter()
	ctx, cancel := context.WithCancel(context.Background())
	err := RegisterGRPCGateway(ctx, zap.NewNop(), router, basePath, lis.Addr().String(), clientTLS, tenancy.NewManager(&tenancyOptions))
	require.NoError(t, err)

	httpLis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	httpServer := &http.Server{
		Handler: router,
	}
	go func() {
		err = httpServer.Serve(httpLis)
		require.Equal(t, http.ErrServerClosed, err)
	}()
	return r, httpLis, grpcServer, cancel, httpServer
}

func testGRPCGatewayWithTenancy(t *testing.T, basePath string, serverTLS tlscfg.Options, clientTLS tlscfg.Options,
	tenancyOptions tenancy.Options,
	setupRequest func(*http.Request),
) {
	defer serverTLS.Close()
	defer clientTLS.Close()

	reader, httpLis, grpcServer, cancel, httpServer := setupGRPCGateway(t, basePath, serverTLS, clientTLS, tenancyOptions)
	defer grpcServer.Stop()
	defer cancel()
	defer httpServer.Shutdown(context.Background())

	traceID := model.NewTraceID(150, 160)
	reader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					TraceID:       traceID,
					SpanID:        model.NewSpanID(180),
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost%s%s/api/v3/traces/123", strings.Replace(httpLis.Addr().String(), "[::]", "", 1), basePath), nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	setupRequest(req)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	buf := bytes.Buffer{}
	_, err = buf.ReadFrom(response.Body)
	require.NoError(t, err)

	jsonpb := &runtime.JSONPb{}
	var envelope envelope
	err = json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, err)
	var spansResponse api_v3.SpansResponseChunk
	err = jsonpb.Unmarshal(envelope.Result, &spansResponse)
	require.NoError(t, err)
	assert.Equal(t, 1, len(spansResponse.GetResourceSpans()))
	assert.Equal(t, uint64ToTraceID(traceID.High, traceID.Low), spansResponse.GetResourceSpans()[0].GetInstrumentationLibrarySpans()[0].GetSpans()[0].GetTraceId())
}

func TestGRPCGateway(t *testing.T) {
	testGRPCGateway(t, "/", tlscfg.Options{}, tlscfg.Options{})
}

func TestGRPCGateway_TLS_with_base_path(t *testing.T) {
	serverTLS := tlscfg.Options{
		Enabled:  true,
		CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
		CertPath: testCertKeyLocation + "/example-server-cert.pem",
		KeyPath:  testCertKeyLocation + "/example-server-key.pem",
	}
	clientTLS := tlscfg.Options{
		Enabled:    true,
		CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
		CertPath:   testCertKeyLocation + "/example-client-cert.pem",
		KeyPath:    testCertKeyLocation + "/example-client-key.pem",
		ServerName: "example.com",
	}
	testGRPCGateway(t, "/jaeger", serverTLS, clientTLS)
}

// For more details why this is needed see https://github.com/grpc-ecosystem/grpc-gateway/issues/2189
type envelope struct {
	Result json.RawMessage `json:"result"`
}

func TestTenancyGRPCGateway(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	tm := tenancy.NewManager(&tenancyOptions)
	testGRPCGatewayWithTenancy(t, "/", tlscfg.Options{}, tlscfg.Options{},
		// Configure the gateway to forward tenancy header from HTTP to GRPC
		tenancyOptions,
		// Add a tenancy header on outbound requests
		func(req *http.Request) {
			req.Header.Add(tm.Header, "dummy")
		})
}

func TestTenancyGRPCRejection(t *testing.T) {
	basePath := "/"
	tenancyOptions := tenancy.Options{Enabled: true}
	reader, httpLis, grpcServer, cancel, httpServer := setupGRPCGateway(t,
		basePath, tlscfg.Options{}, tlscfg.Options{},
		tenancyOptions)
	defer grpcServer.Stop()
	defer cancel()
	defer httpServer.Shutdown(context.Background())

	traceID := model.NewTraceID(150, 160)
	reader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					TraceID:       traceID,
					SpanID:        model.NewSpanID(180),
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost%s%s/api/v3/traces/123", strings.Replace(httpLis.Addr().String(), "[::]", "", 1), basePath), nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// We don't set tenant header
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)

	// Try again with tenant header set
	tm := tenancy.NewManager(&tenancyOptions)
	req.Header.Set(tm.Header, "acme")
	response, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	// Skip unmarshal of response; it is enough that it succeeded
}
