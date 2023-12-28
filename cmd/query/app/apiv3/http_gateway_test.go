// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func setupHTTPGateway(
	t *testing.T,
	basePath string,
	serverTLS, clientTLS *tlscfg.Options,
	tenancyOptions tenancy.Options,
) *testGateway {
	gw := &testGateway{
		reader: &spanstoremocks.Reader{},
	}

	q := querysvc.NewQueryService(gw.reader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)

	hgw := &HTTPGateway{
		QueryService: q,
		TenancyMgr:   tenancy.NewManager(&tenancyOptions),
		Logger:       zap.NewNop(),
		Tracer:       jtracer.NoOp(),
	}

	router := &mux.Router{}
	if basePath != "" && basePath != "/" {
		router = router.PathPrefix(basePath).Subrouter()
	}
	hgw.RegisterRoutes(router)

	httpServer := httptest.NewServer(router)
	t.Cleanup(func() { httpServer.Close() })

	gw.url = httpServer.URL
	if basePath != "/" {
		gw.url += basePath
	}
	return gw
}

func TestHTTPGateway(t *testing.T) {
	useHTTPGateway = true
	t.Cleanup(func() { useHTTPGateway = false })
	t.Run("TestGRPCGateway", TestGRPCGateway)
	t.Run("TestGRPCGatewayWithTenancy", TestGRPCGatewayWithTenancy)
	t.Run("TestGRPCGatewayTenancyRejection", TestGRPCGatewayTenancyRejection)
}

func TestHTTPGatewayTryHandleError(t *testing.T) {
	gw := new(HTTPGateway)
	assert.False(t, gw.tryHandleError(nil, nil, 0), "returns false if no error")

	w := httptest.NewRecorder()
	assert.True(t, gw.tryHandleError(w, spanstore.ErrTraceNotFound, 0), "returns true if error")
	assert.Equal(t, http.StatusNotFound, w.Code, "sets status code to 404")

	logger, log := testutils.NewLogger()
	gw.Logger = logger
	w = httptest.NewRecorder()
	const e = "some err"
	assert.True(t, gw.tryHandleError(w, fmt.Errorf(e), http.StatusInternalServerError))
	assert.Contains(t, log.String(), e, "logs error if status code is 500")
	assert.Contains(t, string(w.Body.String()), e, "writes error message to body")
}

func TestHTTPGatewayGetTraceErrors(t *testing.T) {
	gw := new(HTTPGateway)
	reader := &spanstoremocks.Reader{}
	gw.QueryService = querysvc.NewQueryService(reader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)

	r, err := http.NewRequest(http.MethodGet, "/api/v3/traces/xyz", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	gw.getTrace(w, r)
	assert.Contains(t, w.Body.String(), "malformed trace_id")

	const e = "storage_error"
	reader.
		On("GetTrace", matchContext, matchTraceID).
		Return(nil, fmt.Errorf(e)).Once()

	// TODO this does not work because there is no matcher in the ctx for gorilla
	r, err = http.NewRequest(http.MethodGet, "/api/v3/traces/123", nil)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	gw.getTrace(w, r)
	assert.Contains(t, w.Body.String(), e)
}
