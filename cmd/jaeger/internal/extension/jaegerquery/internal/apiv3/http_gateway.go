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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

// API v3 HTTP query parameter names MUST use camelCase to match the proto3 JSON encoding
// specification (https://protobuf.dev/programming-guides/proto3/#json) and the OpenAPI spec
// (jaeger-idl/swagger/api_v3/query_service.openapi.yaml, generated with naming=json).
//
// DO NOT add new snake_case parameters. New parameters must use the camelCase form of the
// proto field name (e.g., proto field "service_name" → HTTP param "serviceName").
//
// Snake_case forms of existing parameters are preserved below as deprecated aliases.
// Deprecated aliases will be removed in v2.20. See: https://github.com/jaegertracing/jaeger/issues/8619

const (
	// Path variable name for GetTrace (matches google.api.http path template).
	paramTraceID = "trace_id"

	// GetTrace query parameters (canonical).
	paramStartTime = "startTime"
	paramEndTime   = "endTime"
	paramRawTraces = "rawTraces"

	// Deprecated: use startTime. Will be removed in v2.20.
	paramStartTimeDeprecated = "start_time"
	// Deprecated: use endTime. Will be removed in v2.20.
	paramEndTimeDeprecated = "end_time"
	// Deprecated: use rawTraces. Will be removed in v2.20.
	paramRawTracesDeprecated = "raw_traces"

	// FindTraces query parameters (canonical).
	paramServiceName    = "query.serviceName"
	paramOperationName  = "query.operationName"
	paramTimeMin        = "query.startTimeMin"
	paramTimeMax        = "query.startTimeMax"
	paramSearchDepth    = "query.searchDepth"
	paramDurationMin    = "query.durationMin"
	paramDurationMax    = "query.durationMax"
	paramQueryRawTraces = "query.rawTraces"

	// Deprecated: use query.serviceName. Will be removed in v2.20.
	paramServiceNameDeprecated = "query.service_name"
	// Deprecated: use query.operationName. Will be removed in v2.20.
	paramOperationNameDeprecated = "query.operation_name"
	// Deprecated: use query.startTimeMin. Will be removed in v2.20.
	paramTimeMinDeprecated = "query.start_time_min"
	// Deprecated: use query.startTimeMax. Will be removed in v2.20.
	paramTimeMaxDeprecated = "query.start_time_max"
	// Deprecated: use query.searchDepth. Will be removed in v2.20.
	// query.num_traces is a semantic rename from API v2, not a naming-convention alias.
	paramNumTracesDeprecated = "query.num_traces"
	// Deprecated: use query.searchDepth. Will be removed in v2.20.
	paramSearchDepthDeprecated = "query.search_depth"
	// Deprecated: use query.durationMin. Will be removed in v2.20.
	paramDurationMinDeprecated = "query.duration_min"
	// Deprecated: use query.durationMax. Will be removed in v2.20.
	paramDurationMaxDeprecated = "query.duration_max"
	// Deprecated: use query.rawTraces. Will be removed in v2.20.
	paramQueryRawTracesDeprecated = "query.raw_traces"

	// GetOperations query parameters (canonical).
	paramSpanKind = "spanKind"

	// Deprecated: use spanKind. Will be removed in v2.20.
	paramSpanKindDeprecated = "span_kind"

	routeGetTrace      = "/api/v3/traces/{" + paramTraceID + "}"
	routeFindTraces    = "/api/v3/traces"
	routeGetServices   = "/api/v3/services"
	routeGetOperations = "/api/v3/operations"
)

// canonicalQueryParams lists every canonical query parameter name accepted by the HTTP gateway.
// Used by the OpenAPI conformance test.
var canonicalQueryParams = map[string]struct{}{
	paramStartTime:      {},
	paramEndTime:        {},
	paramRawTraces:      {},
	paramServiceName:    {},
	paramOperationName:  {},
	paramTimeMin:        {},
	paramTimeMax:        {},
	paramSearchDepth:    {},
	paramDurationMin:    {},
	paramDurationMax:    {},
	paramQueryRawTraces: {},
	paramSpanKind:       {},
	"service":           {}, // GetOperations required param (proto field name is "service")
}

// HTTPGateway exposes APIv3 HTTP endpoints.
type HTTPGateway struct {
	QueryService *querysvc.QueryService
	Logger       *zap.Logger
	Tracer       trace.TracerProvider
	BasePath     string
}

// RegisterRoutes registers HTTP endpoints for APIv3 into provided mux.
func (h *HTTPGateway) RegisterRoutes(router *http.ServeMux) {
	h.addRoute(router, h.getTrace, routeGetTrace, http.MethodGet)
	h.addRoute(router, h.findTraces, routeFindTraces, http.MethodGet)
	h.addRoute(router, h.getServices, routeGetServices, http.MethodGet)
	h.addRoute(router, h.getOperations, routeGetOperations, http.MethodGet)
}

// addRoute adds a new endpoint to the router with given path and handler function.
func (h *HTTPGateway) addRoute(
	router *http.ServeMux,
	f func(http.ResponseWriter, *http.Request),
	route string,
	method string,
) {
	if h.BasePath != "" && h.BasePath != "/" {
		route = h.BasePath + route
	}
	pattern := method + " " + route
	router.HandleFunc(pattern, f)
}

// emitDeprecation applies deprecation response headers and structured WARN logs when
// deprecated query parameters were used on this request.
func (h *HTTPGateway) emitDeprecation(w http.ResponseWriter, r *http.Request, resolver *paramResolver) {
	if resolver == nil {
		return
	}
	deprecated := resolver.DeprecatedParamsUsed()
	applyDeprecationHeaders(w, deprecated)
	logDeprecatedParams(h.Logger, r.RemoteAddr, deprecated)
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

// tryParamError is similar to tryHandleError but specifically for reporting malformed params.
func (h *HTTPGateway) tryParamError(w http.ResponseWriter, err error, paramName string) bool {
	if err == nil {
		return false
	}
	return h.tryHandleError(w, fmt.Errorf("malformed parameter %s: %w", paramName, err), http.StatusBadRequest)
}

func (h *HTTPGateway) returnTrace(td ptrace.Traces, w http.ResponseWriter) {
	tracesData := jptrace.TracesData(td)
	response := &api_v3.GRPCGatewayWrapper{
		Result: &tracesData,
	}
	h.marshalResponse(response, w)
}

func (h *HTTPGateway) returnTraces(traces []ptrace.Traces, err error, w http.ResponseWriter) {
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	if len(traces) == 0 {
		errorResponse := api_v3.GRPCGatewayError{
			Error: &api_v3.GRPCGatewayError_GRPCGatewayErrorDetails{
				HttpCode: http.StatusNotFound,
				Message:  "No traces found",
			},
		}
		resp, _ := json.Marshal(&errorResponse)
		http.Error(w, string(resp), http.StatusNotFound)
		return
	}
	// TODO: the response should be streamed back to the client
	// https://github.com/jaegertracing/jaeger/issues/6467
	combinedTrace := ptrace.NewTraces()
	for _, t := range traces {
		resources := t.ResourceSpans()
		for i := 0; i < resources.Len(); i++ {
			resource := resources.At(i)
			resource.CopyTo(combinedTrace.ResourceSpans().AppendEmpty())
		}
	}
	h.returnTrace(combinedTrace, w)
}

func (*HTTPGateway) marshalResponse(response proto.Message, w http.ResponseWriter) {
	_ = new(jsonpb.Marshaler).Marshal(w, response)
}

func (h *HTTPGateway) getTrace(w http.ResponseWriter, r *http.Request) {
	resolver := newParamResolver(r)

	traceIDVar := r.PathValue(paramTraceID)
	traceID, err := model.TraceIDFromString(traceIDVar)
	if h.tryParamError(w, err, paramTraceID) {
		return
	}
	request := querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{
				TraceID: v1adapter.FromV1TraceID(traceID),
			},
		},
	}
	if startTime, name, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated); ok {
		timeParsed, err := time.Parse(time.RFC3339Nano, startTime)
		if h.tryParamError(w, err, name) {
			h.emitDeprecation(w, r, resolver)
			return
		}
		request.TraceIDs[0].Start = timeParsed.UTC()
	}
	if endTime, name, ok := resolver.Resolve(paramEndTime, paramEndTimeDeprecated); ok {
		timeParsed, err := time.Parse(time.RFC3339Nano, endTime)
		if h.tryParamError(w, err, name) {
			h.emitDeprecation(w, r, resolver)
			return
		}
		request.TraceIDs[0].End = timeParsed.UTC()
	}
	if raw, name, ok := resolver.Resolve(paramRawTraces, paramRawTracesDeprecated); ok {
		rawTraces, err := strconv.ParseBool(raw)
		if h.tryParamError(w, err, name) {
			h.emitDeprecation(w, r, resolver)
			return
		}
		request.RawTraces = rawTraces
	}
	h.emitDeprecation(w, r, resolver)
	getTracesIter := h.QueryService.GetTraces(r.Context(), request)
	trc, err := jiter.FlattenWithErrors(getTracesIter)
	h.returnTraces(trc, err, w)
}

func (h *HTTPGateway) findTraces(w http.ResponseWriter, r *http.Request) {
	resolver := newParamResolver(r)
	queryParams, shouldReturn := h.parseFindTracesQuery(resolver, w)
	if shouldReturn {
		h.emitDeprecation(w, r, resolver)
		return
	}
	h.emitDeprecation(w, r, resolver)

	findTracesIter := h.QueryService.FindTraces(r.Context(), *queryParams)
	traces, err := jiter.FlattenWithErrors(findTracesIter)
	h.returnTraces(traces, err, w)
}

func (h *HTTPGateway) parseFindTracesQuery(resolver *paramResolver, w http.ResponseWriter) (*querysvc.TraceQueryParams, bool) {
	serviceName, _, _ := resolver.Resolve(paramServiceName, paramServiceNameDeprecated)
	operationName, _, _ := resolver.Resolve(paramOperationName, paramOperationNameDeprecated)

	queryParams := &querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   serviceName,
			OperationName: operationName,
			Attributes:    pcommon.NewMap(), // most curiously not supported by grpc-gateway
		},
	}

	timeMin, timeMinName, timeMinOK := resolver.Resolve(paramTimeMin, paramTimeMinDeprecated)
	timeMax, timeMaxName, timeMaxOK := resolver.Resolve(paramTimeMax, paramTimeMaxDeprecated)
	if !timeMinOK || !timeMaxOK {
		err := fmt.Errorf("%s and %s are required", paramTimeMin, paramTimeMax)
		h.tryHandleError(w, err, http.StatusBadRequest)
		return nil, true
	}
	timeMinParsed, err := time.Parse(time.RFC3339Nano, timeMin)
	if h.tryParamError(w, err, timeMinName) {
		return nil, true
	}
	timeMaxParsed, err := time.Parse(time.RFC3339Nano, timeMax)
	if h.tryParamError(w, err, timeMaxName) {
		return nil, true
	}
	queryParams.StartTimeMin = timeMinParsed
	queryParams.StartTimeMax = timeMaxParsed

	// searchDepth: canonical query.searchDepth; deprecated query.num_traces (v2 semantic)
	// and query.search_depth (snake_case proto field name).
	if n, name, ok := resolver.Resolve(paramSearchDepth, paramNumTracesDeprecated, paramSearchDepthDeprecated); ok {
		searchDepth, err := strconv.Atoi(n)
		if h.tryParamError(w, err, name) {
			return nil, true
		}
		queryParams.SearchDepth = searchDepth
	}

	if d, name, ok := resolver.Resolve(paramDurationMin, paramDurationMinDeprecated); ok {
		dur, err := time.ParseDuration(d)
		if h.tryParamError(w, err, name) {
			return nil, true
		}
		queryParams.DurationMin = dur
	}
	if d, name, ok := resolver.Resolve(paramDurationMax, paramDurationMaxDeprecated); ok {
		dur, err := time.ParseDuration(d)
		if h.tryParamError(w, err, name) {
			return nil, true
		}
		queryParams.DurationMax = dur
	}
	if raw, name, ok := resolver.Resolve(paramQueryRawTraces, paramQueryRawTracesDeprecated); ok {
		rawTraces, err := strconv.ParseBool(raw)
		if h.tryParamError(w, err, name) {
			return nil, true
		}
		queryParams.RawTraces = rawTraces
	}
	return queryParams, false
}

func (h *HTTPGateway) getServices(w http.ResponseWriter, r *http.Request) {
	services, err := h.QueryService.GetServices(r.Context())
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	if services == nil {
		services = []string{}
	}
	h.marshalResponse(&api_v3.GetServicesResponse{
		Services: services,
	}, w)
}

func (h *HTTPGateway) getOperations(w http.ResponseWriter, r *http.Request) {
	resolver := newParamResolver(r)
	spanKind, _, _ := resolver.Resolve(paramSpanKind, paramSpanKindDeprecated)
	queryParams := tracestore.OperationQueryParams{
		ServiceName: resolver.values.Get("service"),
		SpanKind:    spanKind,
	}
	operations, err := h.QueryService.GetOperations(r.Context(), queryParams)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		h.emitDeprecation(w, r, resolver)
		return
	}
	h.emitDeprecation(w, r, resolver)
	apiOperations := make([]*api_v3.Operation, len(operations))
	for i := range operations {
		sk := operations[i].SpanKind
		if sk == "" {
			sk = string(model.SpanKindInternal)
		}
		apiOperations[i] = &api_v3.Operation{
			Name:     operations[i].Name,
			SpanKind: sk,
		}
	}
	h.marshalResponse(&api_v3.GetOperationsResponse{Operations: apiOperations}, w)
}
