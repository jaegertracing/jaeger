// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package analytics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// setup the http gateway
func setupHTTPGatewayNoServer(
	_ *testing.T,
	basePath string,
) *testGateway {
	gw := &testGateway{}

	hgw := &HTTPGateway{
		Logger: zap.NewNop(),
	}

	gw.router = &mux.Router{}
	if basePath != "" && basePath != "/" {
		gw.router = gw.router.PathPrefix(basePath).Subrouter()
	}
	hgw.RegisterRoutes(gw.router)
	return gw
}

func setupHTTPGateway(
	t *testing.T,
	basePath string,
) *testGateway {
	gw := setupHTTPGatewayNoServer(t, basePath)

	httpServer := httptest.NewServer(gw.router)
	t.Cleanup(func() { httpServer.Close() })

	gw.url = httpServer.URL
	if basePath != "/" {
		gw.url += basePath
	}
	return gw
}

func TestHTTPGateway(t *testing.T) {
	runGatewayTests(t, "/", func(_ *http.Request) {})
}
