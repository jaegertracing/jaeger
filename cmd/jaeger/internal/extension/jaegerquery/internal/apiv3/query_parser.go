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
	defaultSearchDepth = 100

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

	timeMinStr, timeMinParam := getQueryParam(q, paramTimeMin, paramTimeMinDeprecated)
	timeMaxStr, timeMaxParam := getQueryParam(q, paramTimeMax, paramTimeMaxDeprecated)
	if timeMinStr == "" || timeMaxStr == "" {
		return nil, fmt.Errorf("%s and %s are required", paramTimeMin, paramTimeMax)
	}
	timeMinParsed, err := time.Parse(time.RFC3339Nano, timeMinStr)
	if err != nil {
		return nil, fmt.Errorf("malformed parameter %s: %w", timeMinParam, err)
	}
	timeMaxParsed, err := time.Parse(time.RFC3339Nano, timeMaxStr)
	if err != nil {
		return nil, fmt.Errorf("malformed parameter %s: %w", timeMaxParam, err)
	}
	if !timeMinParsed.Before(timeMaxParsed) {
		return nil, fmt.Errorf("%s must be before %s", paramTimeMin, paramTimeMax)
	}
	queryParams.StartTimeMin = timeMinParsed
	queryParams.StartTimeMax = timeMaxParsed

	n, searchDepthParam := getQueryParam(q, paramSearchDepth, paramSearchDepthDeprecated)
	if n == "" {
		n = q.Get(paramNumTraces)
		searchDepthParam = paramNumTraces
	}
	if n != "" {
		// bitSize 32 keeps searchDepth inside the protobuf int32 the gRPC
		// storage client later encodes, instead of wrapping on int32(...).
		searchDepth, err := strconv.ParseInt(n, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("malformed parameter %s: %w", searchDepthParam, err)
		}
		queryParams.SearchDepth = int(searchDepth)
	} else {
		queryParams.SearchDepth = defaultSearchDepth
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
