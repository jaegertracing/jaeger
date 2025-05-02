// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"bytes"
	"encoding/json"
	"io"
	"iter"
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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	_ "github.com/jaegertracing/jaeger/internal/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

// Utility functions used from http_gateway_test.go.

const (
	snapshotLocation = "./snapshots/"
)

// Snapshots can be regenerated via:
//
//	REGENERATE_SNAPSHOTS=true go test -v ./cmd/query/app/apiv3/...
var (
	regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"
	traceID             = pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
)

type testGateway struct {
	reader *tracestoremocks.Reader
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

func makeTestTrace() ptrace.Traces {
	trace := ptrace.NewTraces()
	resources := trace.ResourceSpans().AppendEmpty()
	scopes := resources.ScopeSpans().AppendEmpty()

	spanA := scopes.Spans().AppendEmpty()
	spanA.SetName("foobar")
	spanA.SetTraceID(traceID)
	spanA.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))
	spanA.SetKind(ptrace.SpanKindServer)
	spanA.Status().SetCode(ptrace.StatusCodeError)

	return trace
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
	qp := tracestore.OperationQueryParams{ServiceName: "foo", SpanKind: "server"}
	gw.reader.
		On("GetOperations", matchContext, qp).
		Return([]tracestore.Operation{{Name: "get_users", SpanKind: "server"}}, nil).Once()

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
	query := []tracestore.GetTraceParams{{TraceID: traceID}}
	gw.reader.
		On("GetTraces", matchContext, query).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()
	gw.getTracesAndVerify(t, "/api/v3/traces/1", traceID)
}

func (gw *testGateway) runGatewayFindTraces(t *testing.T) {
	q, qp := mockFindQueries()
	gw.reader.On("FindTraces", matchContext, qp).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()
	gw.getTracesAndVerify(t, "/api/v3/traces?"+q.Encode(), traceID)
}

func (gw *testGateway) getTracesAndVerify(t *testing.T, url string, expectedTraceID pcommon.TraceID) {
	body, statusCode := gw.execRequest(t, url)
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))
	body = gw.verifySnapshot(t, body)

	var response api_v3.GRPCGatewayWrapper
	parseResponse(t, body, &response)
	td := response.Result.ToTraces()
	assert.Equal(t, 1, td.SpanCount())
	traceID := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
	assert.Equal(t, expectedTraceID.String(), traceID.String())
}
