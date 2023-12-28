// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
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
	}

	router := &mux.Router{}
	if basePath != "" && basePath != "/" {
		router = router.PathPrefix(basePath).Subrouter()
	}
	hgw.RegisterRoutes(router)

	httpServer := httptest.NewServer(router)
	t.Cleanup(func() { httpServer.Close() })

	t.Logf("HTTP Gateway listening on %s", httpServer.URL)
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
