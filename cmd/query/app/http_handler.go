// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	uiconv "github.com/jaegertracing/jaeger/model/converter/json"
	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	traceIDParam          = "traceID"
	endTsParam            = "endTs"
	lookbackParam         = "lookback"
	stepParam             = "step"
	rateParam             = "ratePer"
	quantileParam         = "quantile"
	groupByOperationParam = "groupByOperation"

	defaultAPIPrefix  = "api"
	prettyPrintIndent = "    "
)

// HTTPHandler handles http requests
type HTTPHandler interface {
	RegisterRoutes(router *mux.Router)
}

type structuredResponse struct {
	Data   any               `json:"data"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Errors []structuredError `json:"errors"`
}

type structuredError struct {
	Code    int        `json:"code,omitempty"`
	Msg     string     `json:"msg"`
	TraceID ui.TraceID `json:"traceID,omitempty"`
}

// NewRouter creates and configures a Gorilla Router.
func NewRouter() *mux.Router {
	return mux.NewRouter().UseEncodedPath()
}

// APIHandler implements the query service public API by registering routes at httpPrefix
type APIHandler struct {
	queryService        *querysvc.QueryService
	metricsQueryService querysvc.MetricsQueryService
	queryParser         queryParser
	basePath            string
	apiPrefix           string
	logger              *zap.Logger
	tracer              trace.TracerProvider
}

// NewAPIHandler returns an APIHandler
func NewAPIHandler(queryService *querysvc.QueryService, options ...HandlerOption) *APIHandler {
	aH := &APIHandler{
		queryService: queryService,
		queryParser: queryParser{
			traceQueryLookbackDuration: defaultTraceQueryLookbackDuration,
			timeNow:                    time.Now,
		},
	}

	for _, option := range options {
		option(aH)
	}
	if aH.apiPrefix == "" {
		aH.apiPrefix = defaultAPIPrefix
	}
	if aH.logger == nil {
		aH.logger = zap.NewNop()
	}
	if aH.tracer == nil {
		aH.tracer = jtracer.NoOp().OTEL
	}
	return aH
}

// RegisterRoutes registers routes for this handler on the given router
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	aH.handleFunc(router, aH.getTrace, "/traces/{%s}", traceIDParam).Methods(http.MethodGet)
	aH.handleFunc(router, aH.archiveTrace, "/archive/{%s}", traceIDParam).Methods(http.MethodPost)
	aH.handleFunc(router, aH.search, "/traces").Methods(http.MethodGet)
	aH.handleFunc(router, aH.getServices, "/services").Methods(http.MethodGet)
	// TODO change the UI to use this endpoint. Requires ?service= parameter.
	aH.handleFunc(router, aH.getOperations, "/operations").Methods(http.MethodGet)
	// TODO - remove this when UI catches up
	aH.handleFunc(router, aH.getOperationsLegacy, "/services/{%s}/operations", serviceParam).Methods(http.MethodGet)
	aH.handleFunc(router, aH.transformOTLP, "/transform").Methods(http.MethodPost)
	aH.handleFunc(router, aH.dependencies, "/dependencies").Methods(http.MethodGet)
	aH.handleFunc(router, aH.latencies, "/metrics/latencies").Methods(http.MethodGet)
	aH.handleFunc(router, aH.calls, "/metrics/calls").Methods(http.MethodGet)
	aH.handleFunc(router, aH.errors, "/metrics/errors").Methods(http.MethodGet)
	aH.handleFunc(router, aH.minStep, "/metrics/minstep").Methods(http.MethodGet)
}

func (aH *APIHandler) handleFunc(
	router *mux.Router,
	f func(http.ResponseWriter, *http.Request),
	routeFmt string,
	args ...any,
) *mux.Route {
	route := aH.formatRoute(routeFmt, args...)
	var handler http.Handler = http.HandlerFunc(f)
	handler = traceResponseHandler(handler)
	handler = otelhttp.WithRouteTag(route, handler)
	handler = spanNameHandler(route, handler)
	return router.HandleFunc(route, handler.ServeHTTP)
}

func (aH *APIHandler) formatRoute(route string, args ...any) string {
	args = append([]any{aH.apiPrefix}, args...)
	return fmt.Sprintf("/%s"+route, args...)
}

func (aH *APIHandler) getServices(w http.ResponseWriter, r *http.Request) {
	services, err := aH.queryService.GetServices(r.Context())
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	structuredRes := structuredResponse{
		Data:  services,
		Total: len(services),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) getOperationsLegacy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	// given how getOperationsLegacy is bound to URL route, serviceParam cannot be empty
	service, _ := url.QueryUnescape(vars[serviceParam])
	// for backwards compatibility, we will retrieve operations with all span kind
	operations, err := aH.queryService.GetOperations(r.Context(),
		spanstore.OperationQueryParameters{
			ServiceName: service,
			// include all kinds
			SpanKind: "",
		})

	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	operationNames := getUniqueOperationNames(operations)
	structuredRes := structuredResponse{
		Data:  operationNames,
		Total: len(operationNames),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) transformOTLP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}

	traces, err := otlp2traces(body)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	var uiErrors []structuredError
	structuredRes := aH.tracesToResponse(traces, false, uiErrors)
	aH.writeJSON(w, r, structuredRes)
}

func (aH *APIHandler) getOperations(w http.ResponseWriter, r *http.Request) {
	service := r.FormValue(serviceParam)
	if service == "" {
		if aH.handleError(w, errServiceParameterRequired, http.StatusBadRequest) {
			return
		}
	}
	spanKind := r.FormValue(spanKindParam)
	operations, err := aH.queryService.GetOperations(
		r.Context(),
		spanstore.OperationQueryParameters{ServiceName: service, SpanKind: spanKind},
	)

	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	data := make([]ui.Operation, len(operations))
	for i, operation := range operations {
		data[i] = ui.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		}
	}
	structuredRes := structuredResponse{
		Data:  data,
		Total: len(operations),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) search(w http.ResponseWriter, r *http.Request) {
	tQuery, err := aH.queryParser.parseTraceQueryParams(r)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}

	var uiErrors []structuredError
	var tracesFromStorage []*model.Trace
	if len(tQuery.traceIDs) > 0 {
		tracesFromStorage, uiErrors, err = aH.tracesByIDs(r.Context(), tQuery.traceIDs)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
	} else {
		tracesFromStorage, err = aH.queryService.FindTraces(r.Context(), &tQuery.TraceQueryParameters)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
	}

	structuredRes := aH.tracesToResponse(tracesFromStorage, true, uiErrors)
	aH.writeJSON(w, r, structuredRes)
}

func (aH *APIHandler) tracesToResponse(traces []*model.Trace, adjust bool, uiErrors []structuredError) *structuredResponse {
	uiTraces := make([]*ui.Trace, len(traces))
	for i, v := range traces {
		uiTrace, uiErr := aH.convertModelToUI(v, adjust)
		if uiErr != nil {
			uiErrors = append(uiErrors, *uiErr)
		}
		uiTraces[i] = uiTrace
	}

	return &structuredResponse{
		Data:   uiTraces,
		Errors: uiErrors,
	}
}

func (aH *APIHandler) tracesByIDs(ctx context.Context, traceIDs []model.TraceID) ([]*model.Trace, []structuredError, error) {
	var traceErrors []structuredError
	retMe := make([]*model.Trace, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		if trc, err := aH.queryService.GetTrace(ctx, traceID); err != nil {
			if !errors.Is(err, spanstore.ErrTraceNotFound) {
				return nil, nil, err
			}
			traceErrors = append(traceErrors, structuredError{
				Msg:     err.Error(),
				TraceID: ui.TraceID(traceID.String()),
			})
		} else {
			retMe = append(retMe, trc)
		}
	}
	return retMe, traceErrors, nil
}

func (aH *APIHandler) dependencies(w http.ResponseWriter, r *http.Request) {
	dqp, err := aH.queryParser.parseDependenciesQueryParams(r)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}
	service := r.FormValue(serviceParam)

	dependencies, err := aH.queryService.GetDependencies(r.Context(), dqp.endTs, dqp.lookback)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	filteredDependencies := aH.filterDependenciesByService(dependencies, service)
	structuredRes := structuredResponse{
		Data: aH.deduplicateDependencies(filteredDependencies),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) latencies(w http.ResponseWriter, r *http.Request) {
	q, err := strconv.ParseFloat(r.FormValue(quantileParam), 64)
	if err != nil {
		aH.handleError(w, newParseError(err, quantileParam), http.StatusBadRequest)
		return
	}
	aH.metrics(w, r, func(ctx context.Context, baseParams metricsstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
		return aH.metricsQueryService.GetLatencies(ctx, &metricsstore.LatenciesQueryParameters{
			BaseQueryParameters: baseParams,
			Quantile:            q,
		})
	})
}

func (aH *APIHandler) calls(w http.ResponseWriter, r *http.Request) {
	aH.metrics(w, r, func(ctx context.Context, baseParams metricsstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
		return aH.metricsQueryService.GetCallRates(ctx, &metricsstore.CallRateQueryParameters{
			BaseQueryParameters: baseParams,
		})
	})
}

func (aH *APIHandler) errors(w http.ResponseWriter, r *http.Request) {
	aH.metrics(w, r, func(ctx context.Context, baseParams metricsstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
		return aH.metricsQueryService.GetErrorRates(ctx, &metricsstore.ErrorRateQueryParameters{
			BaseQueryParameters: baseParams,
		})
	})
}

func (aH *APIHandler) minStep(w http.ResponseWriter, r *http.Request) {
	minStep, err := aH.metricsQueryService.GetMinStepDuration(r.Context(), &metricsstore.MinStepDurationQueryParameters{})
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	structuredRes := structuredResponse{
		Data: minStep.Milliseconds(),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) metrics(w http.ResponseWriter, r *http.Request, getMetrics func(context.Context, metricsstore.BaseQueryParameters) (*metrics.MetricFamily, error)) {
	requestParams, err := aH.queryParser.parseMetricsQueryParams(r)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}
	m, err := getMetrics(r.Context(), requestParams)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	aH.writeJSON(w, r, m)
}

func (aH *APIHandler) convertModelToUI(trc *model.Trace, adjust bool) (*ui.Trace, *structuredError) {
	var errs []error
	if adjust {
		var err error
		trc, err = aH.queryService.Adjust(trc)
		if err != nil {
			errs = append(errs, err)
		}
	}
	uiTrace := uiconv.FromDomain(trc)
	var uiError *structuredError
	if err := errors.Join(errs...); err != nil {
		uiError = &structuredError{
			Msg:     err.Error(),
			TraceID: uiTrace.TraceID,
		}
	}
	return uiTrace, uiError
}

func (*APIHandler) deduplicateDependencies(dependencies []model.DependencyLink) []ui.DependencyLink {
	type Key struct {
		parent string
		child  string
	}
	links := make(map[Key]uint64)

	for _, l := range dependencies {
		links[Key{l.Parent, l.Child}] += l.CallCount
	}

	result := make([]ui.DependencyLink, 0, len(links))
	for k, v := range links {
		result = append(result, ui.DependencyLink{Parent: k.parent, Child: k.child, CallCount: v})
	}

	return result
}

func (*APIHandler) filterDependenciesByService(
	dependencies []model.DependencyLink,
	service string,
) []model.DependencyLink {
	if len(service) == 0 {
		return dependencies
	}

	var filteredDependencies []model.DependencyLink
	for _, dependency := range dependencies {
		if dependency.Parent == service || dependency.Child == service {
			filteredDependencies = append(filteredDependencies, dependency)
		}
	}
	return filteredDependencies
}

// Parses trace ID from URL like /traces/{trace-id}
func (aH *APIHandler) parseTraceID(w http.ResponseWriter, r *http.Request) (model.TraceID, bool) {
	vars := mux.Vars(r)
	traceIDVar := vars[traceIDParam]
	traceID, err := model.TraceIDFromString(traceIDVar)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return traceID, false
	}
	return traceID, true
}

// getTrace implements the REST API /traces/{trace-id}
// It parses trace ID from the path, fetches the trace from QueryService,
// formats it in the UI JSON format, and responds to the client.
func (aH *APIHandler) getTrace(w http.ResponseWriter, r *http.Request) {
	traceID, ok := aH.parseTraceID(w, r)
	if !ok {
		return
	}
	trc, err := aH.queryService.GetTrace(r.Context(), traceID)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		aH.handleError(w, err, http.StatusNotFound)
		return
	}
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	var uiErrors []structuredError
	structuredRes := aH.tracesToResponse([]*model.Trace{trc}, shouldAdjust(r), uiErrors)
	aH.writeJSON(w, r, structuredRes)
}

func shouldAdjust(r *http.Request) bool {
	raw := r.FormValue("raw")
	isRaw, _ := strconv.ParseBool(raw)
	return !isRaw
}

// archiveTrace implements the REST API POST:/archive/{trace-id}.
// It passes the traceID to queryService.ArchiveTrace for writing.
func (aH *APIHandler) archiveTrace(w http.ResponseWriter, r *http.Request) {
	traceID, ok := aH.parseTraceID(w, r)
	if !ok {
		return
	}

	// QueryService.ArchiveTrace can now archive this traceID.
	err := aH.queryService.ArchiveTrace(r.Context(), traceID)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		aH.handleError(w, err, http.StatusNotFound)
		return
	}
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	structuredRes := structuredResponse{
		Data:   []string{}, // doesn't matter, just want an empty array
		Errors: []structuredError{},
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) handleError(w http.ResponseWriter, err error, statusCode int) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, disabled.ErrDisabled) {
		statusCode = http.StatusNotImplemented
	}
	if statusCode == http.StatusInternalServerError {
		aH.logger.Error("HTTP handler, Internal Server Error", zap.Error(err))
	}
	structuredResp := structuredResponse{
		Errors: []structuredError{
			{
				Code: statusCode,
				Msg:  err.Error(),
			},
		},
	}
	resp, _ := json.Marshal(&structuredResp)
	http.Error(w, string(resp), statusCode)
	return true
}

func (aH *APIHandler) writeJSON(w http.ResponseWriter, r *http.Request, response any) {
	prettyPrintValue := r.FormValue(prettyPrintParam)
	prettyPrint := prettyPrintValue != "" && prettyPrintValue != "false"

	var marshal jsonMarshaler
	switch response.(type) {
	case proto.Message:
		marshal = newProtoJSONMarshaler(prettyPrint)
	default:
		marshal = newStructJSONMarshaler(prettyPrint)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := marshal(w, response); err != nil {
		aH.handleError(w, fmt.Errorf("failed writing HTTP response: %w", err), http.StatusInternalServerError)
	}
}

// Returns a handler that generates a traceresponse header.
// https://github.com/w3c/trace-context/blob/main/spec/21-http_response_header_format.md
func traceResponseHandler(handler http.Handler) http.Handler {
	// We use the standard TraceContext propagator, since the formats are identical.
	// But the propagator uses "traceparent" header name, so we inject it into a map
	// `carrier` and then use the result to set the "tracereponse" header.
	var prop propagation.TraceContext
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := make(map[string]string)
		prop.Inject(r.Context(), propagation.MapCarrier(carrier))
		w.Header().Add("traceresponse", carrier["traceparent"])
		handler.ServeHTTP(w, r)
	})
}

func spanNameHandler(spanName string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		span.SetName(spanName)
		handler.ServeHTTP(w, r)
	})
}
