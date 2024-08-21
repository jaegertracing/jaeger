// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/internal/api_v3"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	paramTraceID       = "trace_id"           // get trace by ID
	paramServiceName   = "query.service_name" // find traces
	paramOperationName = "query.operation_name"
	paramTimeMin       = "query.start_time_min"
	paramTimeMax       = "query.start_time_max"
	paramNumTraces     = "query.num_traces"
	paramDurationMin   = "query.duration_min"
	paramDurationMax   = "query.duration_max"

	routeGetTrace      = "/api/v3/traces/{" + paramTraceID + "}"
	routeFindTraces    = "/api/v3/traces"
	routeGetServices   = "/api/v3/services"
	routeGetOperations = "/api/v3/operations"
)

// HTTPGateway exposes APIv3 HTTP endpoints.
type HTTPGateway struct {
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Logger       *zap.Logger
	Tracer       trace.TracerProvider
}

// RegisterRoutes registers HTTP endpoints for APIv3 into provided mux.
// The called can create a subrouter if it needs to prepend a base path.
func (h *HTTPGateway) RegisterRoutes(router *mux.Router) {
	h.addRoute(router, h.getTrace, routeGetTrace).Methods(http.MethodGet)
	h.addRoute(router, h.findTraces, routeFindTraces).Methods(http.MethodGet)
	h.addRoute(router, h.getServices, routeGetServices).Methods(http.MethodGet)
	h.addRoute(router, h.getOperations, routeGetOperations).Methods(http.MethodGet)
}

// addRoute adds a new endpoint to the router with given path and handler function.
// This code is mostly copied from ../http_handler.
func (h *HTTPGateway) addRoute(
	router *mux.Router,
	f func(http.ResponseWriter, *http.Request),
	route string,
	_ ...any, /* args */
) *mux.Route {
	var handler http.Handler = http.HandlerFunc(f)
	if h.TenancyMgr.Enabled {
		handler = tenancy.ExtractTenantHTTPHandler(h.TenancyMgr, handler)
	}
	traceMiddleware := otelhttp.NewHandler(
		otelhttp.WithRouteTag(route, handler),
		route,
		otelhttp.WithTracerProvider(h.Tracer))
	return router.HandleFunc(route, traceMiddleware.ServeHTTP)
}

// tryHandleError checks if the passed error is not nil and handles it by writing
// an error response to the client. Otherwise it returns false.
func (h *HTTPGateway) tryHandleError(w http.ResponseWriter, err error, statusCode int) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		statusCode = http.StatusNotFound
	}
	if statusCode == http.StatusInternalServerError {
		h.Logger.Error("HTTP handler, Internal Server Error", zap.Error(err))
	}
	errorResponse := api_v3.GRPCGatewayError{
		Error: &api_v3.GRPCGatewayError_GRPCGatewayErrorDetails{
			//nolint: gosec // G115
			HttpCode: int32(statusCode),
			Message:  err.Error(),
		},
	}
	resp, _ := json.Marshal(&errorResponse)
	http.Error(w, string(resp), statusCode)
	return true
}

// tryParamError is similar to tryHandleError but specifically for reporting malformed params.
func (h *HTTPGateway) tryParamError(w http.ResponseWriter, err error, paramName string) bool {
	if err == nil {
		return false
	}
	return h.tryHandleError(w, fmt.Errorf("malformed parameter %s: %w", paramName, err), http.StatusBadRequest)
}

func (h *HTTPGateway) returnSpans(spans []*model.Span, w http.ResponseWriter) {
	// modelToOTLP does not easily return an error, so allow mocking it
	h.returnSpansTestable(spans, w, modelToOTLP)
}

func (h *HTTPGateway) returnSpansTestable(
	spans []*model.Span,
	w http.ResponseWriter,
	modelToOTLP func(_ []*model.Span) (ptrace.Traces, error),
) {
	td, err := modelToOTLP(spans)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	tracesData := api_v3.TracesData(td)
	response := &api_v3.GRPCGatewayWrapper{
		Result: &tracesData,
	}
	h.marshalResponse(response, w)
}

func (*HTTPGateway) marshalResponse(response proto.Message, w http.ResponseWriter) {
	_ = new(jsonpb.Marshaler).Marshal(w, response)
}

func (h *HTTPGateway) getTrace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	traceIDVar := vars[paramTraceID]
	traceID, err := model.TraceIDFromString(traceIDVar)
	if h.tryParamError(w, err, paramTraceID) {
		return
	}
	trace, err := h.QueryService.GetTrace(r.Context(), traceID)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	h.returnSpans(trace.Spans, w)
}

func (h *HTTPGateway) findTraces(w http.ResponseWriter, r *http.Request) {
	queryParams, shouldReturn := h.parseFindTracesQuery(r.URL.Query(), w)
	if shouldReturn {
		return
	}

	traces, err := h.QueryService.FindTraces(r.Context(), queryParams)
	// TODO how do we distinguish internal error from bad parameters for FindTrace?
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	var spans []*model.Span
	for _, trace := range traces {
		spans = append(spans, trace.Spans...)
	}
	h.returnSpans(spans, w)
}

func (h *HTTPGateway) parseFindTracesQuery(q url.Values, w http.ResponseWriter) (*spanstore.TraceQueryParameters, bool) {
	queryParams := &spanstore.TraceQueryParameters{
		ServiceName:   q.Get(paramServiceName),
		OperationName: q.Get(paramOperationName),
		Tags:          nil, // most curiously not supported by grpc-gateway
	}

	timeMin := q.Get(paramTimeMin)
	timeMax := q.Get(paramTimeMax)
	if timeMin == "" || timeMax == "" {
		err := fmt.Errorf("%s and %s are required", paramTimeMin, paramTimeMax)
		h.tryHandleError(w, err, http.StatusBadRequest)
		return nil, true
	}
	timeMinParsed, err := time.Parse(time.RFC3339Nano, timeMin)
	if h.tryParamError(w, err, paramTimeMin) {
		return nil, true
	}
	timeMaxParsed, err := time.Parse(time.RFC3339Nano, timeMax)
	if h.tryParamError(w, err, paramTimeMax) {
		return nil, true
	}
	queryParams.StartTimeMin = timeMinParsed
	queryParams.StartTimeMax = timeMaxParsed

	if n := q.Get(paramNumTraces); n != "" {
		numTraces, err := strconv.Atoi(n)
		if h.tryParamError(w, err, paramNumTraces) {
			return nil, true
		}
		queryParams.NumTraces = numTraces
	}

	if d := q.Get(paramDurationMin); d != "" {
		dur, err := time.ParseDuration(d)
		if h.tryParamError(w, err, paramDurationMin) {
			return nil, true
		}
		queryParams.DurationMin = dur
	}
	if d := q.Get(paramDurationMax); d != "" {
		dur, err := time.ParseDuration(d)
		if h.tryParamError(w, err, paramDurationMax) {
			return nil, true
		}
		queryParams.DurationMax = dur
	}
	return queryParams, false
}

func (h *HTTPGateway) getServices(w http.ResponseWriter, r *http.Request) {
	services, err := h.QueryService.GetServices(r.Context())
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	h.marshalResponse(&api_v3.GetServicesResponse{
		Services: services,
	}, w)
}

func (h *HTTPGateway) getOperations(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	queryParams := spanstore.OperationQueryParameters{
		ServiceName: query.Get("service"),
		SpanKind:    query.Get("span_kind"),
	}
	operations, err := h.QueryService.GetOperations(r.Context(), queryParams)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	apiOperations := make([]*api_v3.Operation, len(operations))
	for i := range operations {
		apiOperations[i] = &api_v3.Operation{
			Name:     operations[i].Name,
			SpanKind: operations[i].SpanKind,
		}
	}
	h.marshalResponse(&api_v3.GetOperationsResponse{Operations: apiOperations}, w)
}
