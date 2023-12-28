// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	traceIDParam = "traceID"

	routeGetTrace      = "/api/v3/traces/{traceID}"
	routeFindTraces    = "/api/v3/traces"
	routeGetServices   = "/api/v3/services"
	routeGetOperations = "/api/v3/operations"
)

// HTTPGateway exposes APIv3 HTTP endpoints.
type HTTPGateway struct {
	QueryService *querysvc.QueryService
	TenancyMgr   *tenancy.Manager
	Logger       *zap.Logger
}

// RegisterRoutes registers HTTP endpoints for APIv3 into provided mux.
func (h *HTTPGateway) RegisterRoutes(router *mux.Router) {
	h.addRoute(router, h.getTrace, routeGetTrace).Methods(http.MethodGet)
	h.addRoute(router, h.findTraces, routeFindTraces).Methods(http.MethodGet)
	h.addRoute(router, h.getServices, routeGetServices).Methods(http.MethodGet)
	h.addRoute(router, h.getOperations, routeGetOperations).Methods(http.MethodGet)
}

// addRoute adds a new endpoint to the router with given path and handler function.
// This code is mostly copied from ../http_handler.
// TODO add tracing middleware.
func (h *HTTPGateway) addRoute(
	router *mux.Router,
	f func(http.ResponseWriter, *http.Request),
	route string,
	args ...interface{},
) *mux.Route {
	// route := aH.formatRoute(routeFmt, args...)
	var handler http.Handler = http.HandlerFunc(f)
	if h.TenancyMgr.Enabled {
		handler = tenancy.ExtractTenantHTTPHandler(h.TenancyMgr, handler)
	}
	// traceMiddleware := otelhttp.NewHandler(
	// 	otelhttp.WithRouteTag(route, traceResponseHandler(handler)),
	// 	route,
	// 	otelhttp.WithTracerProvider(aH.tracer.OTEL))
	// return router.HandleFunc(route, traceMiddleware.ServeHTTP)
	return router.HandleFunc(route, handler.ServeHTTP)
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
			HttpCode: int32(statusCode),
			Message:  err.Error(),
		},
	}
	resp, _ := json.Marshal(&errorResponse)
	http.Error(w, string(resp), statusCode)
	return true
}

func (h *HTTPGateway) returnSpans(spans []*model.Span, w http.ResponseWriter) {
	resourceSpans, err := modelToOTLP(spans)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	for _, rs := range resourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				if len(s.ParentSpanId) == 0 {
					// If ParentSpanId is empty array then gogo/jsonpb renders it as empty string.
					// To match the output with grpc-gateway we set it to nil and it won't be included.
					s.ParentSpanId = nil
				}
			}
		}
	}
	response := &api_v3.GRPCGatewayWrapper{
		Result: &api_v3.SpansResponseChunk{
			ResourceSpans: resourceSpans,
		},
	}

	h.marshalResponse(response, w)
}

func (h *HTTPGateway) marshalResponse(response proto.Message, w http.ResponseWriter) {
	m := &jsonpb.Marshaler{
		EmitDefaults: false,
	}
	_ = m.Marshal(w, response)
}

func (h *HTTPGateway) getTrace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	traceIDVar := vars[traceIDParam]
	traceID, err := model.TraceIDFromString(traceIDVar)
	if h.tryHandleError(w, err, http.StatusBadRequest) {
		return
	}
	trace, err := h.QueryService.GetTrace(r.Context(), traceID)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	h.returnSpans(trace.Spans, w)
}

func (h *HTTPGateway) findTraces(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	queryParams := &spanstore.TraceQueryParameters{
		ServiceName:   query.Get("query.service_name"),
		OperationName: query.Get("query.operation_name"),
		Tags:          nil, // most curiously not supported by grpc-gateway
	}
	if n := query.Get("query.num_traces"); n != "" {
		numTraces, err := strconv.Atoi(n)
		if h.tryHandleError(w, err, http.StatusBadRequest) {
			return
		}
		queryParams.NumTraces = numTraces
	}
	timeMin := query.Get("query.start_time_min")
	timeMax := query.Get("query.start_time_max")
	if timeMin == "" || timeMax == "" {
		err := fmt.Errorf("query.start_time_min and query.start_time_max are required")
		h.tryHandleError(w, err, http.StatusBadRequest)
		return
	}
	timeMinParsed, err := time.Parse(time.RFC3339Nano, timeMin)
	if h.tryHandleError(w, err, http.StatusBadRequest) {
		return
	}
	queryParams.StartTimeMin = timeMinParsed
	timeMaxParsed, err := time.Parse(time.RFC3339Nano, timeMax)
	if h.tryHandleError(w, err, http.StatusBadRequest) {
		return
	}
	queryParams.StartTimeMax = timeMaxParsed
	if d := query.Get("duration_min"); d != "" {
		dur, err := time.ParseDuration(d)
		if h.tryHandleError(w, err, http.StatusBadRequest) {
			return
		}
		queryParams.DurationMin = dur
	}
	if d := query.Get("duration_max"); d != "" {
		dur, err := time.ParseDuration(d)
		if h.tryHandleError(w, err, http.StatusBadRequest) {
			return
		}
		queryParams.DurationMax = dur
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
