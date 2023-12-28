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
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
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
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

const (
	testCertKeyLocation = "../../../../pkg/config/tlscfg/testdata/"
	snapshotLocation    = "./snapshots/"
)

// Snapshots can be regenerated via:
//
//	REGENERATE_SNAPSHOTS=true go test -v ./cmd/query/app/apiv3/...
var regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

// The tests in http_gateway_test.go set this to true to use manual gateway implementation.
var useHTTPGateway = false

type testGateway struct {
	reader *spanstoremocks.Reader
	url    string
}

type gatewayRequest struct {
	url          string
	setupRequest func(*http.Request)
}

func setupGRPCGateway(
	t *testing.T,
	basePath string,
	serverTLS, clientTLS *tlscfg.Options,
	tenancyOptions tenancy.Options,
) *testGateway {
	if useHTTPGateway {
		return setupHTTPGateway(t, basePath, serverTLS, clientTLS, tenancyOptions)
	}
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

func (gw *testGateway) execRequest(t *testing.T, gwReq *gatewayRequest) ([]byte, int) {
	req, err := http.NewRequest(http.MethodGet, gw.url+gwReq.url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	gwReq.setupRequest(req)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return body, response.StatusCode
}

func verifySnapshot(t *testing.T, body []byte) []byte {
	// reformat JSON body with indentation, to make diffing easier
	var data interface{}
	require.NoError(t, json.Unmarshal(body, &data), "response: %s", string(body))
	body, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	testName := path.Base(t.Name())
	snapshotFile := filepath.Join(snapshotLocation, testName+".json")
	if regenerateSnapshots {
		os.WriteFile(snapshotFile, body, 0o644)
	}
	snapshot, err := os.ReadFile(snapshotFile)
	require.NoError(t, err)
	assert.Equal(t, string(snapshot), string(body), "comparing against stored snapshot. Use REGENERATE_SNAPSHOTS=true to rebuild snapshots.")
	return body
}

func parseResponse(t *testing.T, body []byte, obj gogoproto.Message) {
	require.NoError(t, gogojsonpb.Unmarshal(bytes.NewBuffer(body), obj))
}

func makeTestTrace() (*model.Trace, model.TraceID) {
	traceID := model.NewTraceID(150, 160)
	return &model.Trace{
		Spans: []*model.Span{
			{
				TraceID:       traceID,
				SpanID:        model.NewSpanID(180),
				OperationName: "foobar",
			},
		},
	}, traceID
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
	t.Run("GetServices", func(t *testing.T) {
		runGatewayGetServices(t, gw, setupRequest)
	})
	t.Run("GetOperations", func(t *testing.T) {
		runGatewayGetOperations(t, gw, setupRequest)
	})
	t.Run("GetTrace", func(t *testing.T) {
		runGatewayGetTrace(t, gw, setupRequest)
	})
	t.Run("FindTraces", func(t *testing.T) {
		runGatewayFindTraces(t, gw, setupRequest)
	})
}

func runGatewayGetServices(t *testing.T, gw *testGateway, setupRequest func(*http.Request)) {
	gw.reader.On("GetServices", matchContext).Return([]string{"foo"}, nil).Once()

	body, statusCode := gw.execRequest(t, &gatewayRequest{
		url:          "/api/v3/services",
		setupRequest: setupRequest,
	})
	require.Equal(t, http.StatusOK, statusCode)
	body = verifySnapshot(t, body)

	var response api_v3.GetServicesResponse
	parseResponse(t, body, &response)
	assert.Equal(t, []string{"foo"}, response.Services)
}

func runGatewayGetOperations(t *testing.T, gw *testGateway, setupRequest func(*http.Request)) {
	qp := spanstore.OperationQueryParameters{ServiceName: "foo", SpanKind: "server"}
	gw.reader.
		On("GetOperations", matchContext, qp).
		Return([]spanstore.Operation{{Name: "get_users", SpanKind: "server"}}, nil).Once()

	body, statusCode := gw.execRequest(t, &gatewayRequest{
		url:          "/api/v3/operations?service=foo&span_kind=server",
		setupRequest: setupRequest,
	})
	require.Equal(t, http.StatusOK, statusCode)
	body = verifySnapshot(t, body)

	var response api_v3.GetOperationsResponse
	parseResponse(t, body, &response)
	require.Len(t, response.Operations, 1)
	assert.Equal(t, "get_users", response.Operations[0].Name)
	assert.Equal(t, "server", response.Operations[0].SpanKind)
}

func runGatewayGetTrace(t *testing.T, gw *testGateway, setupRequest func(*http.Request)) {
	trace, traceID := makeTestTrace()
	gw.reader.On("GetTrace", matchContext, traceID).Return(trace, nil).Once()

	body, statusCode := gw.execRequest(t, &gatewayRequest{
		url:          "/api/v3/traces/" + traceID.String(), // hex string
		setupRequest: setupRequest,
	})
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))
	body = verifySnapshot(t, body)

	var response api_v3.GRPCGatewayWrapper
	parseResponse(t, body, &response)

	assert.Len(t, response.Result.ResourceSpans, 1)
	assert.Equal(t,
		bytesOfTraceID(t, traceID.High, traceID.Low),
		response.Result.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
}

func runGatewayFindTraces(t *testing.T, gw *testGateway, setupRequest func(*http.Request)) {
	trace, traceID := makeTestTrace()
	gw.reader.
		On("FindTraces", matchContext, mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{trace}, nil).Once()

	q := url.Values{}
	q.Set("query.service_name", "foobar")
	q.Set("query.start_time_min", time.Now().Format(time.RFC3339))
	q.Set("query.start_time_max", time.Now().Format(time.RFC3339))

	body, statusCode := gw.execRequest(t, &gatewayRequest{
		url:          "/api/v3/traces?" + q.Encode(),
		setupRequest: setupRequest,
	})
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))
	body = verifySnapshot(t, body)

	var response api_v3.GRPCGatewayWrapper
	parseResponse(t, body, &response)

	assert.Len(t, response.Result.ResourceSpans, 1)
	assert.Equal(t,
		bytesOfTraceID(t, traceID.High, traceID.Low),
		response.Result.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
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

func TestGRPCGatewayTenancyRejection(t *testing.T) {
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
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusUnauthorized, response.StatusCode, "response=%s", string(body))

	// Try again with tenant header set
	tm := tenancy.NewManager(&tenancyOptions)
	req.Header.Set(tm.Header, "acme")
	response, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	// Skip unmarshal of response; it is enough that it succeeded
}
