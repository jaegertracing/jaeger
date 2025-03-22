// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	v2querysvc "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/internal/config"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	bearerToken  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbiIsIm5hbWUiOiJKb2huIERvZSIsImlhdCI"
	bearerHeader = "Bearer " + bearerToken
)

type elasticsearchHandlerMock struct {
	test *testing.T
}

func (*elasticsearchHandlerMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if token, ok := bearertoken.GetBearerToken(r.Context()); ok && token == bearerToken {
		// Return empty results, we don't care about the result here.
		// we just need to make sure the token was propagated to the storage and the query-service returns 200
		ret := new(elastic.SearchResult)
		json_ret, _ := json.Marshal(ret)
		w.Header().Add("Content-Type", "application/json; charset=UTF-8")
		w.Write(json_ret)
		return
	}

	// No token, return error!
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

func runMockElasticsearchServer(t *testing.T) *httptest.Server {
	handler := &elasticsearchHandlerMock{
		test: t,
	}
	return httptest.NewServer(
		bearertoken.PropagationHandler(zaptest.NewLogger(t), handler),
	)
}

func runQueryService(t *testing.T, esURL string) *Server {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	flagsSvc.Logger = zaptest.NewLogger(t)

	telset := telemetry.NoopSettings()
	telset.Logger = flagsSvc.Logger
	telset.ReportStatus = telemetry.HCAdapter(flagsSvc.HC())

	f := es.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	require.NoError(t, command.ParseFlags([]string{
		"--es.tls.enabled=false",
		"--es.version=7",
		"--es.server-urls=" + esURL,
	}))
	f.InitFromViper(v, flagsSvc.Logger)
	// set AllowTokenFromContext manually because we don't register the respective CLI flag from query svc
	f.Options.Config.Authentication.BearerTokenAuthentication.AllowFromContext = true
	require.NoError(t, f.Initialize(telset.Metrics, telset.Logger))
	defer f.Close()

	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	traceReader := v1adapter.NewTraceReader(spanReader)

	querySvc := querysvc.NewQueryService(traceReader, nil, querysvc.QueryServiceOptions{})
	v2QuerySvc := v2querysvc.NewQueryService(traceReader, nil, v2querysvc.QueryServiceOptions{})
	server, err := NewServer(context.Background(), querySvc, v2QuerySvc, nil,
		&QueryOptions{
			BearerTokenPropagation: true,
			HTTP: confighttp.ServerConfig{
				Endpoint: ":0",
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":0",
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		tenancy.NewManager(&tenancy.Options{}),
		telset,
	)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))
	return server
}

func TestBearerTokenPropagation(t *testing.T) {
	testCases := []struct {
		name        string
		headerValue string
		headerName  string
	}{
		{name: "Bearer token", headerName: "Authorization", headerValue: bearerHeader},
		{name: "Raw Bearer token", headerName: "Authorization", headerValue: bearerToken},
		{name: "X-Forwarded-Access-Token", headerName: "X-Forwarded-Access-Token", headerValue: bearerHeader},
	}

	esSrv := runMockElasticsearchServer(t)
	defer esSrv.Close()
	t.Logf("mock ES server started on %s", esSrv.URL)

	querySrv := runQueryService(t, esSrv.URL)
	defer querySrv.Close()
	queryAddr := querySrv.httpConn.Addr().String()
	// Will try to load service names, this should return 200.
	url := fmt.Sprintf("http://%s/api/services", queryAddr)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)
			req.Header.Add(testCase.headerName, testCase.headerValue)

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
