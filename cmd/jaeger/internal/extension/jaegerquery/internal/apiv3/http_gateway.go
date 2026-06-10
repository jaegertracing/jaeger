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

const (
	routeGetTrace        = "/api/v3/traces/{" + paramTraceID + "}"
	routeFindTraces      = "/api/v3/traces"
	routeFindSummaries   = "/api/v3/trace-summaries"
	routeGetServices     = "/api/v3/services"
	routeGetOperations   = "/api/v3/operations"
	routeGetDependencies = "/api/v3/dependencies"
)

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
	h.addRoute(router, h.findTraceSummaries, routeFindSummaries, http.MethodGet)
	h.addRoute(router, h.getServices, routeGetServices, http.MethodGet)
	h.addRoute(router, h.getOperations, routeGetOperations, http.MethodGet)
	h.addRoute(router, h.getDependencies, routeGetDependencies, http.MethodGet)
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
	w.Header().Set("Content-Type", "application/json")
	_ = new(jsonpb.Marshaler).Marshal(w, response)
}

func (h *HTTPGateway) getTrace(w http.ResponseWriter, r *http.Request) {
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
	q := r.URL.Query()
	if startTime, paramName := getQueryParam(q, paramStartTime, paramStartTimeDeprecated); startTime != "" {
		timeParsed, err := time.Parse(time.RFC3339Nano, startTime)
		if h.tryParamError(w, err, paramName) {
			return
		}
		request.TraceIDs[0].Start = timeParsed.UTC()
	}
	if endTime, paramName := getQueryParam(q, paramEndTime, paramEndTimeDeprecated); endTime != "" {
		timeParsed, err := time.Parse(time.RFC3339Nano, endTime)
		if h.tryParamError(w, err, paramName) {
			return
		}
		request.TraceIDs[0].End = timeParsed.UTC()
	}
	if rawStr, paramName := getQueryParam(q, paramRawTraces, paramRawTracesDeprecated); rawStr != "" {
		rawTraces, err := strconv.ParseBool(rawStr)
		if h.tryParamError(w, err, paramName) {
			return
		}
		request.RawTraces = rawTraces
	}
	getTracesIter := h.QueryService.GetTraces(r.Context(), request)
	trc, err := jiter.FlattenWithErrors(getTracesIter)
	h.returnTraces(trc, err, w)
}

func (h *HTTPGateway) findTraces(w http.ResponseWriter, r *http.Request) {
	queryParams, err := parseFindTracesQuery(r.URL.Query())
	if h.tryHandleError(w, err, http.StatusBadRequest) {
		return
	}

	findTracesIter := h.QueryService.FindTraces(r.Context(), *queryParams)
	traces, err := jiter.FlattenWithErrors(findTracesIter)
	h.returnTraces(traces, err, w)
}

func (h *HTTPGateway) findTraceSummaries(w http.ResponseWriter, r *http.Request) {
	queryParams, err := parseFindTracesQuery(r.URL.Query())
	if h.tryHandleError(w, err, http.StatusBadRequest) {
		return
	}
	// Summaries always use adjusted, aggregated data; raw_traces has no effect here.
	queryParams.RawTraces = false
	summariesIter := h.QueryService.FindTraceSummaries(r.Context(), *queryParams)
	summaries, err := jiter.FlattenWithErrors(summariesIter)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	h.marshalResponse(&api_v3.FindTraceSummariesResponse{
		Summaries: toProtoTraceSummaries(summaries),
	}, w)
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
	q := r.URL.Query()
	spanKind, _ := getQueryParam(q, paramSpanKind, paramSpanKindDeprecated)
	queryParams := tracestore.OperationQueryParams{
		ServiceName: q.Get("service"),
		SpanKind:    spanKind,
	}
	operations, err := h.QueryService.GetOperations(r.Context(), queryParams)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	apiOperations := make([]*api_v3.Operation, len(operations))
	for i := range operations {
		spanKind := operations[i].SpanKind
		if spanKind == "" {
			spanKind = string(model.SpanKindInternal)
		}
		apiOperations[i] = &api_v3.Operation{
			Name:     operations[i].Name,
			SpanKind: spanKind,
		}
	}
	h.marshalResponse(&api_v3.GetOperationsResponse{Operations: apiOperations}, w)
}

func (h *HTTPGateway) getDependencies(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	startTimeStr, _ := getQueryParam(q, paramStartTime, paramStartTimeDeprecated)
	endTimeStr, _ := getQueryParam(q, paramEndTime, paramEndTimeDeprecated)

	if startTimeStr == "" {
		h.tryHandleError(w, fmt.Errorf("missing required parameter: %s", paramStartTime), http.StatusBadRequest)
		return
	}
	if endTimeStr == "" {
		h.tryHandleError(w, fmt.Errorf("missing required parameter: %s", paramEndTime), http.StatusBadRequest)
		return
	}

	startTime, err := time.Parse(time.RFC3339Nano, startTimeStr)
	if h.tryParamError(w, err, paramStartTime) {
		return
	}

	endTime, err := time.Parse(time.RFC3339Nano, endTimeStr)
	if h.tryParamError(w, err, paramEndTime) {
		return
	}

	if !endTime.After(startTime) {
		h.tryHandleError(w, fmt.Errorf("%s must be after %s", paramEndTime, paramStartTime), http.StatusBadRequest)
		return
	}

	lookback := endTime.Sub(startTime)
	dependencies, err := h.QueryService.GetDependencies(r.Context(), endTime, lookback)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}

	response := &api_v3.DependenciesResponse{
		Dependencies: make([]*api_v3.Dependency, 0, len(dependencies)),
	}
	for _, dep := range dependencies {
		response.Dependencies = append(response.Dependencies, &api_v3.Dependency{
			Parent:    dep.Parent,
			Child:     dep.Child,
			CallCount: dep.CallCount,
		})
	}
	h.marshalResponse(response, w)
}
