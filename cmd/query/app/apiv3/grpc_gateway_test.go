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
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" //force gogo codec registration
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var testCertKeyLocation = "../../../../pkg/config/tlscfg/testdata/"

func TestGRPCGateway(t *testing.T) {
	r := &spanstoremocks.Reader{}
	traceID := model.NewTraceID(150, 160)
	r.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					TraceID:       traceID,
					SpanID:        model.NewSpanID(180),
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	grpcServer := grpc.NewServer()
	h := &Handler{
		QueryService: q,
	}
	api_v3.RegisterQueryServiceServer(grpcServer, h)
	lis, _ := net.Listen("tcp", ":0")
	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()
	defer grpcServer.GracefulStop()

	router := &mux.Router{}
	err := RegisterGRPCGateway(zap.NewNop(), router, "", lis.Addr().String(), tlscfg.Options{})
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
	defer httpServer.Shutdown(context.Background())
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost%s/v3/traces/123", strings.Replace(httpLis.Addr().String(), "[::]", "", 1)), nil)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
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

func TestGRPCGateway_TLS(t *testing.T) {
	r := &spanstoremocks.Reader{}
	traceID := model.NewTraceID(150, 160)
	r.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					TraceID:       traceID,
					SpanID:        model.NewSpanID(180),
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})

	tlsOpts := tlscfg.Options{
		Enabled:  true,
		CAPath:   testCertKeyLocation + "/example-CA-cert.pem",
		CertPath: testCertKeyLocation + "/example-server-cert.pem",
		KeyPath:  testCertKeyLocation + "/example-server-key.pem",
	}
	config, err := tlsOpts.Config(zap.NewNop())
	require.NoError(t, err)

	creds := credentials.NewTLS(config)
	grpcServer := grpc.NewServer([]grpc.ServerOption{grpc.Creds(creds)}...)
	h := &Handler{
		QueryService: q,
	}
	api_v3.RegisterQueryServiceServer(grpcServer, h)
	lis, _ := net.Listen("tcp", ":0")
	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()
	defer grpcServer.GracefulStop()

	router := &mux.Router{}
	err = RegisterGRPCGateway(zap.NewNop(), router, "", lis.Addr().String(), tlscfg.Options{
		Enabled:    true,
		CAPath:     testCertKeyLocation + "/example-CA-cert.pem",
		CertPath:   testCertKeyLocation + "/example-client-cert.pem",
		KeyPath:    testCertKeyLocation + "/example-client-key.pem",
		ServerName: "example.com",
	})
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
	defer httpServer.Shutdown(context.Background())
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost%s/v3/traces/123", strings.Replace(httpLis.Addr().String(), "[::]", "", 1)), nil)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	buf := bytes.Buffer{}
	_, err = buf.ReadFrom(response.Body)
	require.NoError(t, err)

	jsonpb := &runtime.JSONPb{}
	var envelope envelope
	err = json.Unmarshal(buf.Bytes(), &envelope)
	require.NoError(t, err)
	var spansResponse api_v3.SpansResponseChunk
	fmt.Println(string(buf.Bytes()))
	err = jsonpb.Unmarshal(envelope.Result, &spansResponse)
	require.NoError(t, err)
	assert.Equal(t, 1, len(spansResponse.GetResourceSpans()))
	assert.Equal(t, uint64ToTraceID(traceID.High, traceID.Low), spansResponse.GetResourceSpans()[0].GetInstrumentationLibrarySpans()[0].GetSpans()[0].GetTraceId())
}

// see https://github.com/grpc-ecosystem/grpc-gateway/issues/2189
type envelope struct {
	Result json.RawMessage `json:"result"`
}
