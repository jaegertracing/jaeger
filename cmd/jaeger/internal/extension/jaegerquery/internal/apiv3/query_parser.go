// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

const (
	paramTraceID = "trace_id" // path parameter

	// Canonical camelCase query params matching proto3 JSON encoding.
	paramStartTime      = "startTime"
	paramEndTime        = "endTime"
	paramRawTraces      = "rawTraces"
	paramServiceName    = "query.serviceName"
	paramOperationName  = "query.operationName"
	paramTimeMin        = "query.startTimeMin"
	paramTimeMax        = "query.startTimeMax"
	paramSearchDepth    = "query.searchDepth"
	paramDurationMin    = "query.durationMin"
	paramDurationMax    = "query.durationMax"
	paramQueryRawTraces = "query.rawTraces"
	paramSpanKind       = "spanKind"

	// Deprecated snake_case aliases kept for backward compatibility.
	paramStartTimeDeprecated      = "start_time"
	paramEndTimeDeprecated        = "end_time"
	paramRawTracesDeprecated      = "raw_traces"
	paramServiceNameDeprecated    = "query.service_name"
	paramOperationNameDeprecated  = "query.operation_name"
	paramTimeMinDeprecated        = "query.start_time_min"
	paramTimeMaxDeprecated        = "query.start_time_max"
	paramSearchDepthDeprecated    = "query.search_depth"
	paramNumTraces                = "query.num_traces" // deprecated alias for paramSearchDepth
	paramDurationMinDeprecated    = "query.duration_min"
	paramDurationMaxDeprecated    = "query.duration_max"
	paramQueryRawTracesDeprecated = "query.raw_traces"
	paramSpanKindDeprecated       = "span_kind"

	routeGetTrace      = "/api/v3/traces/{" + paramTraceID + "}"
	routeFindTraces    = "/api/v3/traces"
	routeFindSummaries = "/api/v3/trace-summaries"
	routeGetServices   = "/api/v3/services"
	routeGetOperations = "/api/v3/operations"
)

// getQueryParam returns the value and effective param name, preferring the canonical name
// and falling back to the deprecated alias.
func getQueryParam(q url.Values, canonical, deprecated string) (value string, paramName string) {
	if v := q.Get(canonical); v != "" {
		return v, canonical
	}
	return q.Get(deprecated), deprecated
}

func (h *HTTPGateway) parseFindTracesQuery(q url.Values, w http.ResponseWriter) (*querysvc.TraceQueryParams, bool) {
	serviceName, _ := getQueryParam(q, paramServiceName, paramServiceNameDeprecated)
	operationName, _ := getQueryParam(q, paramOperationName, paramOperationNameDeprecated)
	queryParams := &querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   serviceName,
			OperationName: operationName,
			Attributes:    pcommon.NewMap(), // most curiously not supported by grpc-gateway
		},
	}

	timeMinStr, timeMinParam := getQueryParam(q, paramTimeMin, paramTimeMinDeprecated)
	timeMaxStr, timeMaxParam := getQueryParam(q, paramTimeMax, paramTimeMaxDeprecated)
	if timeMinStr == "" || timeMaxStr == "" {
		err := fmt.Errorf("%s and %s are required", paramTimeMin, paramTimeMax)
		h.tryHandleError(w, err, http.StatusBadRequest)
		return nil, true
	}
	timeMinParsed, err := time.Parse(time.RFC3339Nano, timeMinStr)
	if h.tryParamError(w, err, timeMinParam) {
		return nil, true
	}
	timeMaxParsed, err := time.Parse(time.RFC3339Nano, timeMaxStr)
	if h.tryParamError(w, err, timeMaxParam) {
		return nil, true
	}
	queryParams.StartTimeMin = timeMinParsed
	queryParams.StartTimeMax = timeMaxParsed

	n, searchDepthParam := getQueryParam(q, paramSearchDepth, paramSearchDepthDeprecated)
	if n == "" {
		n = q.Get(paramNumTraces)
		searchDepthParam = paramNumTraces
	}
	if n != "" {
		searchDepth, err := strconv.Atoi(n)
		if h.tryParamError(w, err, searchDepthParam) {
			return nil, true
		}
		queryParams.SearchDepth = searchDepth
	}

	if d, paramName := getQueryParam(q, paramDurationMin, paramDurationMinDeprecated); d != "" {
		dur, err := time.ParseDuration(d)
		if h.tryParamError(w, err, paramName) {
			return nil, true
		}
		queryParams.DurationMin = dur
	}
	if d, paramName := getQueryParam(q, paramDurationMax, paramDurationMaxDeprecated); d != "" {
		dur, err := time.ParseDuration(d)
		if h.tryParamError(w, err, paramName) {
			return nil, true
		}
		queryParams.DurationMax = dur
	}
	if r, paramName := getQueryParam(q, paramQueryRawTraces, paramQueryRawTracesDeprecated); r != "" {
		rawTraces, err := strconv.ParseBool(r)
		if h.tryParamError(w, err, paramName) {
			return nil, true
		}
		queryParams.RawTraces = rawTraces
	}
	return queryParams, false
}
