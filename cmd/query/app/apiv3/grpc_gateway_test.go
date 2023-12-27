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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/stretchr/testify/assert"
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

type testGateway struct {
	reader *spanstoremocks.Reader
	url    string
}

func setupGRPCGateway(
	t *testing.T,
	basePath string,
	serverTLS, clientTLS *tlscfg.Options,
	tenancyOptions tenancy.Options,
) *testGateway {
	// *spanstoremocks.Reader, net.Listener, *grpc.Server, context.CancelFunc, *http.Server
	gw := &testGateway{
		reader: &spanstoremocks.Reader{},
	}

	q := querysvc.NewQueryService(gw.reader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)

	var serverGRPCOpts []grpc.ServerOption
	if serverTLS.Enabled {
		config, err := serverTLS.Config(zap.NewNop())
		require.NoError(t, err)
		t.Cleanup(func() { serverTLS.Close() })
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
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()
	t.Cleanup(func() { grpcServer.Stop() })

	router := &mux.Router{}
	router = router.PathPrefix(basePath).Subrouter()
	ctx, cancel := context.WithCancel(context.Background())
	err = RegisterGRPCGateway(
		ctx, zap.NewNop(), router, basePath,
		lis.Addr().String(), clientTLS, tenancy.NewManager(&tenancyOptions),
	)
	require.NoError(t, err)
	t.Cleanup(func() { cancel() })
	t.Cleanup(func() { clientTLS.Close() })

	httpLis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	httpServer := &http.Server{
		Handler: router,
	}
	go func() {
		err = httpServer.Serve(httpLis)
		require.Equal(t, http.ErrServerClosed, err)
	}()
	t.Cleanup(func() { httpServer.Shutdown(context.Background()) })

	gw.url = fmt.Sprintf(
		"http://localhost%s%s",
		strings.Replace(httpLis.Addr().String(), "[::]", "", 1),
		basePath)
	return gw
}

func testGRPCGateway(
	t *testing.T, basePath string,
	serverTLS, clientTLS *tlscfg.Options,
) {
	testGRPCGatewayWithTenancy(t, basePath, serverTLS, clientTLS,
		tenancy.Options{
			Enabled: false,
		},
		func(*http.Request) { /* setupRequest : no changes */ },
	)
}

func testGRPCGatewayWithTenancy(
	t *testing.T,
	basePath string,
	serverTLS *tlscfg.Options,
	clientTLS *tlscfg.Options,
	tenancyOptions tenancy.Options,
	setupRequest func(*http.Request),
) {
	gw := setupGRPCGateway(t, basePath, serverTLS, clientTLS, tenancyOptions)

	traceID := model.NewTraceID(150, 160)
	gw.reader.On("GetTrace", matchContext, matchTraceID).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					TraceID:       traceID,
					SpanID:        model.NewSpanID(180),
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	req, err := http.NewRequest(http.MethodGet, gw.url+"/api/v3/traces/123", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	setupRequest(req)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	jsonpb := &runtime.JSONPb{}
	var envelope envelope
	err = json.Unmarshal(body, &envelope)
	require.NoError(t, err)
	var spansResponse api_v3.SpansResponseChunk
	err = jsonpb.Unmarshal(envelope.Result, &spansResponse)
	require.NoError(t, err)
	assert.Len(t, spansResponse.GetResourceSpans(), 1)
	assert.Equal(t, bytesOfTraceID(t, traceID.High, traceID.Low), spansResponse.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()[0].GetTraceId())
}

func bytesOfTraceID(t *testing.T, high, low uint64) []byte {
	traceID := model.NewTraceID(high, low)
	buf := make([]byte, 16)
	_, err := traceID.MarshalTo(buf)
	require.NoError(t, err)
	return buf
}

func TestGRPCGateway(t *testing.T) {
	testGRPCGateway(t, "/", &tlscfg.Options{}, &tlscfg.Options{})
}

func TestGRPCGatewayWithBasePathAndTLS(t *testing.T) {
	serverTLS := &tlscfg.Options{
		Enabled:  true,
		CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
		CertPath: testCertKeyLocation + "/example-server-cert.pem",
		KeyPath:  testCertKeyLocation + "/example-server-key.pem",
	}
	clientTLS := &tlscfg.Options{
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

func TestGRPCGatewayWithTenancy(t *testing.T) {
	tenancyOptions := tenancy.Options{
		Enabled: true,
	}
	tm := tenancy.NewManager(&tenancyOptions)
	testGRPCGatewayWithTenancy(t, "/", &tlscfg.Options{}, &tlscfg.Options{},
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
	gw := setupGRPCGateway(t,
		basePath, &tlscfg.Options{}, &tlscfg.Options{},
		tenancyOptions)

	traceID := model.NewTraceID(150, 160)
	gw.reader.On("GetTrace", matchContext, matchTraceID).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					TraceID:       traceID,
					SpanID:        model.NewSpanID(180),
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	req, err := http.NewRequest(http.MethodGet, gw.url+"/api/v3/traces/123", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// We don't set tenant header
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusForbidden, response.StatusCode)

	// Try again with tenant header set
	tm := tenancy.NewManager(&tenancyOptions)
	req.Header.Set(tm.Header, "acme")
	response, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	// Skip unmarshal of response; it is enough that it succeeded
}
