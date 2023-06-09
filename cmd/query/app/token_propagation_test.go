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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	bearerToken  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbiIsIm5hbWUiOiJKb2huIERvZSIsImlhdCI"
	bearerHeader = "Bearer " + bearerToken
)

type elasticsearchHandlerMock struct {
	test *testing.T
}

func (h *elasticsearchHandlerMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	f := es.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	require.NoError(t, command.ParseFlags([]string{
		"--es.tls.enabled=false",
		"--es.version=7",
		"--es.server-urls=" + esURL,
	}))
	f.InitFromViper(v, flagsSvc.Logger)
	// set AllowTokenFromContext manually because we don't register the respective CLI flag from query svc
	f.Options.Primary.AllowTokenFromContext = true
	require.NoError(t, f.Initialize(metrics.NullFactory, flagsSvc.Logger))

	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)

	querySvc := querysvc.NewQueryService(spanReader, nil, querysvc.QueryServiceOptions{})
	server, err := NewServer(flagsSvc.Logger, querySvc, nil,
		&QueryOptions{GRPCHostPort: ":0", HTTPHostPort: ":0", BearerTokenPropagation: true},
		tenancy.NewManager(&tenancy.Options{}),
		jtracer.NoOp(),
	)
	require.NoError(t, err)
	require.NoError(t, server.Start())
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

			assert.Equal(t, resp.StatusCode, http.StatusOK)
		})
	}
}
