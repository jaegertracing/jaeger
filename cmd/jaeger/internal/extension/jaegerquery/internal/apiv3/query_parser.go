// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
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
	paramAttributes     = "query.attributes"
	paramFilter         = "query.filter"
	paramSpanKind       = "spanKind"
	paramPageSize       = "query.pagination.pageSize"
	paramPageToken      = "query.pagination.pageToken"

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
)

// parseTimeQueryParam parses value, already resolved from a query parameter, as RFC3339Nano,
// reporting a malformed value under paramName. An empty value is not an error: it returns the
// zero time, so a caller can assign the result unconditionally.
func parseTimeQueryParam(value, paramName string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("malformed parameter %s: %w", paramName, err)
	}
	return parsed, nil
}

// getQueryParam returns the value and effective param name, preferring the canonical name
// and falling back to the deprecated alias.
func getQueryParam(q url.Values, canonical, deprecated string) (value string, paramName string) {
	if v := q.Get(canonical); v != "" {
		return v, canonical
	}
	return q.Get(deprecated), deprecated
}

func parseFindTracesQuery(q url.Values) (*querysvc.TraceQueryParams, error) {
	serviceName, _ := getQueryParam(q, paramServiceName, paramServiceNameDeprecated)
	operationName, _ := getQueryParam(q, paramOperationName, paramOperationNameDeprecated)

	queryParams := &querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   serviceName,
			OperationName: operationName,
			Attributes:    pcommon.NewMap(),
		},
	}
	if attrsParam := q.Get(paramAttributes); attrsParam != "" {
		var attrsMap map[string]string
		if err := json.Unmarshal([]byte(attrsParam), &attrsMap); err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramAttributes, err)
		}
		queryParams.Attributes = jptrace.PlainMapToPcommonMap(attrsMap)
	}
	// The filter parameter carries a JSON-encoded expression.
	if filterParam := q.Get(paramFilter); filterParam != "" {
		var call expressionproto.Call
		if err := jsonpb.Unmarshal(strings.NewReader(filterParam), &call); err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramFilter, err)
		}
		filter, err := expressionproto.FromProto(&call)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramFilter, err)
		}
		queryParams.Filter = filter
	}

	// The parser reads each parameter and reports one it cannot read under its own name. Whether
	// the query as a whole is acceptable (a present and ordered time range, a bounded search
	// depth) is the query service's decision, so it is not repeated here.
	minStr, minParam := getQueryParam(q, paramTimeMin, paramTimeMinDeprecated)
	startTimeMin, err := parseTimeQueryParam(minStr, minParam)
	if err != nil {
		return nil, err
	}
	queryParams.StartTimeMin = startTimeMin
	maxStr, maxParam := getQueryParam(q, paramTimeMax, paramTimeMaxDeprecated)
	startTimeMax, err := parseTimeQueryParam(maxStr, maxParam)
	if err != nil {
		return nil, err
	}
	queryParams.StartTimeMax = startTimeMax

	n, searchDepthParam := getQueryParam(q, paramSearchDepth, paramSearchDepthDeprecated)
	if n == "" {
		n = q.Get(paramNumTraces)
		searchDepthParam = paramNumTraces
	}
	if n != "" {
		searchDepth, err := strconv.ParseInt(n, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", searchDepthParam, err)
		}
		queryParams.SearchDepth = int(searchDepth)
	}

	if d, paramName := getQueryParam(q, paramDurationMin, paramDurationMinDeprecated); d != "" {
		dur, err := time.ParseDuration(d)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramName, err)
		}
		queryParams.DurationMin = dur
	}
	if d, paramName := getQueryParam(q, paramDurationMax, paramDurationMaxDeprecated); d != "" {
		dur, err := time.ParseDuration(d)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramName, err)
		}
		queryParams.DurationMax = dur
	}
	if r, paramName := getQueryParam(q, paramQueryRawTraces, paramQueryRawTracesDeprecated); r != "" {
		rawTraces, err := strconv.ParseBool(r)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramName, err)
		}
		queryParams.RawTraces = rawTraces
	}
	return queryParams, nil
}

// parseFindSpansQuery parses the query parameters for a span search (RFC 0016 §4.3). The parser
// reads each parameter and reports one it cannot read under its own name; whether the query as a
// whole is acceptable (a present and ordered time range, pagination) is the query service's
// decision (prepareSpanSearchQuery), so it is not repeated here.
func parseFindSpansQuery(q url.Values) (*querysvc.SpanQueryParams, error) {
	queryParams := &querysvc.SpanQueryParams{}

	// This is a new endpoint with no callers to keep the deprecated snake_case aliases for, so
	// it reads only the canonical camelCase params.
	startTimeMin, err := parseTimeQueryParam(q.Get(paramTimeMin), paramTimeMin)
	if err != nil {
		return nil, err
	}
	queryParams.StartTimeMin = startTimeMin
	startTimeMax, err := parseTimeQueryParam(q.Get(paramTimeMax), paramTimeMax)
	if err != nil {
		return nil, err
	}
	queryParams.StartTimeMax = startTimeMax

	if filterParam := q.Get(paramFilter); filterParam != "" {
		var call expressionproto.Call
		if err := jsonpb.Unmarshal(strings.NewReader(filterParam), &call); err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramFilter, err)
		}
		filter, err := expressionproto.FromProto(&call)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", paramFilter, err)
		}
		queryParams.Filter = filter
	}

	queryParams.Pagination.PageToken = q.Get(paramPageToken)
	if pageSizeStr := q.Get(paramPageSize); pageSizeStr != "" {
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil || pageSize < 0 {
			return nil, fmt.Errorf("malformed parameter %s: %s", paramPageSize, pageSizeStr)
		}
		queryParams.Pagination.PageSize = pageSize
	}
	return queryParams, nil
}
