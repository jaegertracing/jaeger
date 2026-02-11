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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	deepdependencies "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/ddg"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/qualitymetrics"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	ui "github.com/jaegertracing/jaeger/internal/uimodel"
	uiconv "github.com/jaegertracing/jaeger/internal/uimodel/converter/v1/json"
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
	RegisterRoutes(router *http.ServeMux)
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

// APIHandler implements the query service public API by registering routes at httpPrefix
type APIHandler struct {
	queryService        *querysvc.QueryService
	metricsQueryService metricstore.Reader
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
		aH.tracer = nooptrace.NewTracerProvider()
	}
	return aH
}

// RegisterRoutes registers routes for this handler on the given router
func (aH *APIHandler) RegisterRoutes(router *http.ServeMux) {
	aH.handleFunc(router, aH.getTrace, http.MethodGet, "/traces/{%s}", traceIDParam)
	aH.handleFunc(router, aH.archiveTrace, http.MethodPost, "/archive/{%s}", traceIDParam)
	aH.handleFunc(router, aH.search, http.MethodGet, "/traces")
	aH.handleFunc(router, aH.getServices, http.MethodGet, "/services")
	// TODO change the UI to use this endpoint. Requires ?service= parameter.
	aH.handleFunc(router, aH.getOperations, http.MethodGet, "/operations")
	// TODO - remove this when UI catches up
	aH.handleFunc(router, aH.getOperationsLegacy, http.MethodGet, "/services/{%s}/operations", serviceParam)
	aH.handleFunc(router, aH.transformOTLP, http.MethodPost, "/transform")
	aH.handleFunc(router, aH.dependencies, http.MethodGet, "/dependencies")
	aH.handleFunc(router, aH.deepDependencies, http.MethodGet, "/deep-dependencies")
	aH.handleFunc(router, aH.latencies, http.MethodGet, "/metrics/latencies")
	aH.handleFunc(router, aH.calls, http.MethodGet, "/metrics/calls")
	aH.handleFunc(router, aH.errors, http.MethodGet, "/metrics/errors")
	aH.handleFunc(router, aH.minStep, http.MethodGet, "/metrics/minstep")
	aH.handleFunc(router, aH.getQualityMetrics, http.MethodGet, "/quality-metrics")
}

func (aH *APIHandler) handleFunc(
	router *http.ServeMux,
	f func(http.ResponseWriter, *http.Request),
	method string,
	routeFmt string,
	args ...any,
) {
	route := aH.formatRoute(routeFmt, args...)
	pattern := method + " " + route
	router.HandleFunc(pattern, f)
}

func (aH *APIHandler) formatRoute(route string, args ...any) string {
	args = append([]any{aH.apiPrefix}, args...)
	formattedRoute := fmt.Sprintf("/%s"+route, args...)
	if aH.basePath != "" && aH.basePath != "/" {
		formattedRoute = aH.basePath + formattedRoute
	}
	return formattedRoute
}

func (aH *APIHandler) getServices(w http.ResponseWriter, r *http.Request) {
	services, err := aH.queryService.GetServices(r.Context())
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	if len(services) == 0 {
		services = []string{}
	}

	structuredRes := structuredResponse{
		Data:  services,
		Total: len(services),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) getOperationsLegacy(w http.ResponseWriter, r *http.Request) {
	// given how getOperationsLegacy is bound to URL route, serviceParam cannot be empty
	service, _ := url.QueryUnescape(r.PathValue(serviceParam))
	// for backwards compatibility, we will retrieve operations with all span kind
	operations, err := aH.queryService.GetOperations(r.Context(),
		tracestore.OperationQueryParams{
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
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}

	var uiErrors []structuredError
	structuredRes := aH.tracesToResponse(traces, uiErrors)
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
		tracestore.OperationQueryParams{ServiceName: service, SpanKind: spanKind},
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
	if len(tQuery.TraceIDs) > 0 {
		tracesFromStorage, uiErrors, err = aH.tracesByIDs(
			r.Context(),
			tQuery,
		)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
	} else {
		// Convert to v2 query params and call v2 QueryService
		queryParams := querysvc.TraceQueryParams{
			TraceQueryParams: tQuery.TraceQueryParams,
			RawTraces:        tQuery.RawTraces,
		}
		findTracesIter := aH.queryService.FindTraces(r.Context(), queryParams)
		tracesFromStorage, err = v1adapter.V1TracesFromSeq2(findTracesIter)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
	}

	structuredRes := aH.tracesToResponse(tracesFromStorage, uiErrors)
	aH.writeJSON(w, r, structuredRes)
}

func (*APIHandler) tracesToResponse(traces []*model.Trace, uiErrors []structuredError) *structuredResponse {
	uiTraces := make([]*ui.Trace, len(traces))
	for i, v := range traces {
		uiTrace := uiconv.FromDomain(v)
		uiTraces[i] = uiTrace
	}

	return &structuredResponse{
		Data:   uiTraces,
		Errors: uiErrors,
	}
}

func (aH *APIHandler) tracesByIDs(ctx context.Context, traceQuery *traceQueryParameters) ([]*model.Trace, []structuredError, error) {
	var traceErrors []structuredError
	retMe := make([]*model.Trace, 0, len(traceQuery.TraceIDs))
	if len(traceQuery.TraceIDs) == 0 {
		return nil, nil, nil
	}

	ids := make([]tracestore.GetTraceParams, len(traceQuery.TraceIDs))
	requestedIDs := make(map[pcommon.TraceID]model.TraceID)
	for i, traceID := range traceQuery.TraceIDs {
		v2ID := v1adapter.FromV1TraceID(traceID)
		ids[i] = tracestore.GetTraceParams{
			TraceID: v2ID,
			Start:   traceQuery.StartTimeMin,
			End:     traceQuery.StartTimeMax,
		}
		requestedIDs[v2ID] = traceID
	}

	query := querysvc.GetTraceParams{
		TraceIDs:  ids,
		RawTraces: traceQuery.RawTraces,
	}

	getTracesIter := aH.queryService.GetTraces(ctx, query)
	traces, err := v1adapter.V1TracesFromSeq2(getTracesIter)
	if err != nil {
		return nil, nil, err
	}

	foundIDs := make(map[pcommon.TraceID]struct{})
	for _, tr := range traces {
		if len(tr.Spans) > 0 {
			retMe = append(retMe, tr)
			foundIDs[v1adapter.FromV1TraceID(tr.Spans[0].TraceID)] = struct{}{}
		}
	}

	for v2ID, v1ID := range requestedIDs {
		if _, ok := foundIDs[v2ID]; !ok {
			traceErrors = append(traceErrors, structuredError{
				Msg:     spanstore.ErrTraceNotFound.Error(),
				TraceID: ui.TraceID(v1ID.String()),
			})
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

func (aH *APIHandler) deepDependencies(w http.ResponseWriter, r *http.Request) {
	focalService := r.URL.Query().Get("service")
	data := deepdependencies.GetData(focalService)
	aH.writeJSON(w, r, &data)
}

func (aH *APIHandler) latencies(w http.ResponseWriter, r *http.Request) {
	q, err := strconv.ParseFloat(r.FormValue(quantileParam), 64)
	if err != nil {
		aH.handleError(w, newParseError(err, quantileParam), http.StatusBadRequest)
		return
	}
	aH.metrics(w, r, func(ctx context.Context, baseParams metricstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
		return aH.metricsQueryService.GetLatencies(ctx, &metricstore.LatenciesQueryParameters{
			BaseQueryParameters: baseParams,
			Quantile:            q,
		})
	})
}

func (aH *APIHandler) calls(w http.ResponseWriter, r *http.Request) {
	aH.metrics(w, r, func(ctx context.Context, baseParams metricstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
		return aH.metricsQueryService.GetCallRates(ctx, &metricstore.CallRateQueryParameters{
			BaseQueryParameters: baseParams,
		})
	})
}

func (aH *APIHandler) errors(w http.ResponseWriter, r *http.Request) {
	aH.metrics(w, r, func(ctx context.Context, baseParams metricstore.BaseQueryParameters) (*metrics.MetricFamily, error) {
		return aH.metricsQueryService.GetErrorRates(ctx, &metricstore.ErrorRateQueryParameters{
			BaseQueryParameters: baseParams,
		})
	})
}

func (aH *APIHandler) minStep(w http.ResponseWriter, r *http.Request) {
	minStep, err := aH.metricsQueryService.GetMinStepDuration(r.Context(), &metricstore.MinStepDurationQueryParameters{})
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	structuredRes := structuredResponse{
		Data: minStep.Milliseconds(),
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) metrics(w http.ResponseWriter, r *http.Request, getMetrics func(context.Context, metricstore.BaseQueryParameters) (*metrics.MetricFamily, error)) {
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
	if service == "" {
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
	traceIDVar := r.PathValue(traceIDParam)
	traceID, err := model.TraceIDFromString(traceIDVar)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return traceID, false
	}
	return traceID, true
}

func (aH *APIHandler) parseMicroseconds(w http.ResponseWriter, r *http.Request, timeKey string) (time.Time, bool) {
	if timeString := r.FormValue(timeKey); timeString != "" {
		t, err := aH.queryParser.parseTime(r, timeKey, time.Microsecond)
		if aH.handleError(w, err, http.StatusBadRequest) {
			return time.Time{}, false
		}
		return t, true
	}
	// It's OK if no time is specified, since it's optional
	return time.Time{}, true
}

func (aH *APIHandler) parseBool(w http.ResponseWriter, r *http.Request, boolKey string) (value bool, isValid bool) {
	if boolString := r.FormValue(boolKey); boolString != "" {
		b, err := parseBool(r, boolKey)
		if aH.handleError(w, err, http.StatusBadRequest) {
			return false, false
		}
		return b, true
	}
	return false, true
}

func (aH *APIHandler) parseGetTraceParameters(w http.ResponseWriter, r *http.Request) (querysvc.GetTraceParams, bool) {
	var query querysvc.GetTraceParams
	traceID, ok := aH.parseTraceID(w, r)
	if !ok {
		return query, false
	}
	startTime, ok := aH.parseMicroseconds(w, r, startTimeParam)
	if !ok {
		return query, false
	}
	endTime, ok := aH.parseMicroseconds(w, r, endTimeParam)
	if !ok {
		return query, false
	}
	raw, ok := aH.parseBool(w, r, rawParam)
	if !ok {
		return query, false
	}
	query.TraceIDs = []tracestore.GetTraceParams{
		{
			TraceID: v1adapter.FromV1TraceID(traceID),
			Start:   startTime,
			End:     endTime,
		},
	}
	query.RawTraces = raw
	return query, true
}

// getTrace implements the REST API /traces/{trace-id}
// It parses trace ID from the path, fetches the trace from QueryService,
// formats it in the UI JSON format, and responds to the client.
func (aH *APIHandler) getTrace(w http.ResponseWriter, r *http.Request) {
	query, ok := aH.parseGetTraceParameters(w, r)
	if !ok {
		return
	}
	getTracesIter := aH.queryService.GetTraces(r.Context(), query)
	traces, err := v1adapter.V1TracesFromSeq2(getTracesIter)
	if errors.Is(err, spanstore.ErrTraceNotFound) || (err == nil && len(traces) == 0) {
		aH.handleError(w, spanstore.ErrTraceNotFound, http.StatusNotFound)
		return
	}
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	var uiErrors []structuredError
	structuredRes := aH.tracesToResponse(traces, uiErrors)
	aH.writeJSON(w, r, structuredRes)
}

// archiveTrace implements the REST API POST:/archive/{trace-id}.
// It passes the traceID to queryService.ArchiveTrace for writing.
func (aH *APIHandler) archiveTrace(w http.ResponseWriter, r *http.Request) {
	query, ok := aH.parseGetTraceParameters(w, r)
	if !ok {
		return
	}

	// QueryService.ArchiveTrace can now archive this traceID.
	err := aH.queryService.ArchiveTrace(r.Context(), query.TraceIDs[0])
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

func (aH *APIHandler) getQualityMetrics(w http.ResponseWriter, r *http.Request) {
	data := qualitymetrics.GetSampleData()
	aH.writeJSON(w, r, &data)
}
