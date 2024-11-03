// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/query/app/internal/api_v3"
	"github.com/jaegertracing/jaeger/model"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

// Utility functions used from http_gateway_test.go.

const (
	snapshotLocation = "./snapshots/"
)

// Snapshots can be regenerated via:
//
//	REGENERATE_SNAPSHOTS=true go test -v ./cmd/query/app/apiv3/...
var regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

type testGateway struct {
	reader *spanstoremocks.Reader
	url    string
	router *mux.Router
	// used to set a tenancy header when executing requests
	setupRequest func(*http.Request)
}

func (gw *testGateway) execRequest(t *testing.T, url string) ([]byte, int) {
	req, err := http.NewRequest(http.MethodGet, gw.url+url, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	gw.setupRequest(req)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return body, response.StatusCode
}

func (*testGateway) verifySnapshot(t *testing.T, body []byte) []byte {
	// reformat JSON body with indentation, to make diffing easier
	var data any
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

func makeTestTrace() (*model.Trace, spanstore.TraceGetParameters) {
	traceID := model.NewTraceID(150, 160)
	query := spanstore.TraceGetParameters{TraceID: traceID}
	return &model.Trace{
		Spans: []*model.Span{
			{
				TraceID:       traceID,
				SpanID:        model.NewSpanID(180),
				OperationName: "foobar",
				Tags: []model.KeyValue{
					model.String("span.kind", "server"),
					model.Bool("error", true),
				},
			},
		},
	}, query
}

func runGatewayTests(
	t *testing.T,
	basePath string,
	setupRequest func(*http.Request),
) {
	gw := setupHTTPGateway(t, basePath)
	gw.setupRequest = setupRequest
	t.Run("GetServices", gw.runGatewayGetServices)
	t.Run("GetOperations", gw.runGatewayGetOperations)
	t.Run("GetTrace", gw.runGatewayGetTrace)
	t.Run("FindTraces", gw.runGatewayFindTraces)
}

func (gw *testGateway) runGatewayGetServices(t *testing.T) {
	gw.reader.On("GetServices", matchContext).Return([]string{"foo"}, nil).Once()

	body, statusCode := gw.execRequest(t, "/api/v3/services")
	require.Equal(t, http.StatusOK, statusCode)
	body = gw.verifySnapshot(t, body)

	var response api_v3.GetServicesResponse
	parseResponse(t, body, &response)
	assert.Equal(t, []string{"foo"}, response.Services)
}

func (gw *testGateway) runGatewayGetOperations(t *testing.T) {
	qp := spanstore.OperationQueryParameters{ServiceName: "foo", SpanKind: "server"}
	gw.reader.
		On("GetOperations", matchContext, qp).
		Return([]spanstore.Operation{{Name: "get_users", SpanKind: "server"}}, nil).Once()

	body, statusCode := gw.execRequest(t, "/api/v3/operations?service=foo&span_kind=server")
	require.Equal(t, http.StatusOK, statusCode)
	body = gw.verifySnapshot(t, body)

	var response api_v3.GetOperationsResponse
	parseResponse(t, body, &response)
	require.Len(t, response.Operations, 1)
	assert.Equal(t, "get_users", response.Operations[0].Name)
	assert.Equal(t, "server", response.Operations[0].SpanKind)
}

func (gw *testGateway) runGatewayGetTrace(t *testing.T) {
	trace, query := makeTestTrace()
	gw.reader.On("GetTrace", matchContext, query).Return(trace, nil).Once()
	gw.getTracesAndVerify(t, "/api/v3/traces/"+query.TraceID.String(), query.TraceID)
}

func (gw *testGateway) runGatewayFindTraces(t *testing.T) {
	trace, query := makeTestTrace()
	q, qp := mockFindQueries()
	gw.reader.
		On("FindTraces", matchContext, qp).
		Return([]*model.Trace{trace}, nil).Once()
	gw.getTracesAndVerify(t, "/api/v3/traces?"+q.Encode(), query.TraceID)
}

func (gw *testGateway) getTracesAndVerify(t *testing.T, url string, expectedTraceID model.TraceID) {
	body, statusCode := gw.execRequest(t, url)
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))
	body = gw.verifySnapshot(t, body)

	var response api_v3.GRPCGatewayWrapper
	parseResponse(t, body, &response)
	td := response.Result.ToTraces()
	assert.EqualValues(t, 1, td.SpanCount())
	traceID := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
	assert.Equal(t, expectedTraceID.String(), traceID.String())
}
