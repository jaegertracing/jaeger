// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	defaultQueryLimit = 100

	operationParam   = "operation"
	tagParam         = "tag"
	tagsParam        = "tags"
	startTimeParam   = "start"
	limitParam       = "limit"
	minDurationParam = "minDuration"
	maxDurationParam = "maxDuration"
	serviceParam     = "service"
	prettyPrintParam = "prettyPrint"
	endTimeParam     = "end"

	// servicesParam refers to the path segment of the metrics query endpoint containing the list of comma-separated
	// services to request metrics for.
	// For example, for the metrics request URL `http://localhost:16686/api/metrics/calls/emailservice,frontend`
	// the "call rate" metrics for the following services will be returned: "frontend" and "emailservice".
	servicesParam = "services"

	// spanKindsParam refers to the path segment of the metrics query endpoint containing the list of comma-separated
	// span kinds to filter on for the metrics query.
	// For example, for the metrics request URL `http://localhost:16686/api/metrics/calls/emailservice?spanKinds=SPAN_KIND_SERVER,SPAN_KIND_CLIENT`
	// the "call rate" metrics for the "emailservice" service with span kind of either "server" or "client" will be returned.
	// Note the use of the string representation of span kinds based on the OpenTelemetry proto data model.
	spanKindsParam = "spanKinds"
)

var (
	errMaxDurationGreaterThanMin = fmt.Errorf("'%s' should be greater than '%s'", maxDurationParam, minDurationParam)

	// ErrServiceParameterRequired occurs when no service name is defined
	ErrServiceParameterRequired = fmt.Errorf("parameter '%s' is required", serviceParam)

	msDuration = time.Millisecond
)

// queryParser handles the parsing of query parameters for traces
type queryParser struct {
	traceQueryLookbackDuration time.Duration
	timeNow                    func() time.Time
}

type traceQueryParameters struct {
	spanstore.TraceQueryParameters
	traceIDs []model.TraceID
}

// parseTraceQueryParams takes a request and constructs a model of parameters
// Trace query syntax:
//     query ::= param | param '&' query
//     param ::= service | operation | limit | start | end | minDuration | maxDuration | tag | tags
//     service ::= 'service=' strValue
//     operation ::= 'operation=' strValue
//     limit ::= 'limit=' intValue
//     start ::= 'start=' intValue in unix microseconds
//     end ::= 'end=' intValue in unix microseconds
//     minDuration ::= 'minDuration=' strValue (units are "ns", "us" (or "µs"), "ms", "s", "m", "h")
//     maxDuration ::= 'maxDuration=' strValue (units are "ns", "us" (or "µs"), "ms", "s", "m", "h")
//     tag ::= 'tag=' key | 'tag=' keyvalue
//     key := strValue
//     keyValue := strValue ':' strValue
//     tags :== 'tags=' jsonMap
func (p *queryParser) parseTraceQueryParams(r *http.Request) (*traceQueryParameters, error) {
	service := r.FormValue(serviceParam)
	operation := r.FormValue(operationParam)

	startTime, err := p.parseTime(r, startTimeParam, time.Microsecond)
	if err != nil {
		return nil, err
	}
	endTime, err := p.parseTime(r, endTimeParam, time.Microsecond)
	if err != nil {
		return nil, err
	}

	tags, err := p.parseTags(r.Form[tagParam], r.Form[tagsParam])
	if err != nil {
		return nil, err
	}

	limitParam := r.FormValue(limitParam)
	limit := defaultQueryLimit
	if limitParam != "" {
		limitParsed, err := strconv.ParseInt(limitParam, 10, 32)
		if err != nil {
			return nil, err
		}
		limit = int(limitParsed)
	}

	minDuration, err := p.parseDuration(r, minDurationParam, nil, 0)
	if err != nil {
		return nil, err
	}

	maxDuration, err := p.parseDuration(r, maxDurationParam, nil, 0)
	if err != nil {
		return nil, err
	}

	var traceIDs []model.TraceID
	for _, id := range r.Form[traceIDParam] {
		if traceID, err := model.TraceIDFromString(id); err == nil {
			traceIDs = append(traceIDs, traceID)
		} else {
			return nil, fmt.Errorf("cannot parse traceID param: %w", err)
		}
	}

	traceQuery := &traceQueryParameters{
		TraceQueryParameters: spanstore.TraceQueryParameters{
			ServiceName:   service,
			OperationName: operation,
			StartTimeMin:  startTime,
			StartTimeMax:  endTime,
			Tags:          tags,
			NumTraces:     limit,
			DurationMin:   minDuration,
			DurationMax:   maxDuration,
		},
		traceIDs: traceIDs,
	}

	if err := p.validateQuery(traceQuery); err != nil {
		return nil, err
	}
	return traceQuery, nil
}

func (p *queryParser) parseMetricsQueryParams(r *http.Request) (bqp metricsstore.BaseQueryParameters, err error) {
	serviceNames := r.FormValue(servicesParam)
	if serviceNames == "" {
		return bqp, newParseError(errors.New("please provide at least one service name"), servicesParam)
	}
	bqp.ServiceNames = strings.Split(serviceNames, ",")

	bqp.GroupByOperation, err = p.parseBool(r, groupByOperationParam)
	if err != nil {
		return bqp, err
	}
	bqp.SpanKinds, err = p.parseSpanKinds(r, spanKindsParam, defaultMetricsSpanKinds)
	if err != nil {
		return bqp, err
	}
	endTs, err := p.parseTime(r, endTsParam, msDuration)
	if err != nil {
		return bqp, err
	}
	lookback, err := p.parseDuration(r, lookbackParam, &msDuration, defaultMetricsQueryLookbackDuration)
	if err != nil {
		return bqp, err
	}
	step, err := p.parseDuration(r, stepParam, &msDuration, defaultMetricsQueryStepDuration)
	if err != nil {
		return bqp, err
	}
	ratePer, err := p.parseDuration(r, rateParam, &msDuration, defaultMetricsQueryRateDuration)
	if err != nil {
		return bqp, err
	}
	bqp.EndTime = &endTs
	bqp.Lookback = &lookback
	bqp.Step = &step
	bqp.RatePer = &ratePer
	return bqp, err
}

// parseTime parses the time parameter of an HTTP request that is represented the number of "units" since epoch.
// If the time parameter is empty, the current time will be returned.
func (p *queryParser) parseTime(r *http.Request, paramName string, units time.Duration) (time.Time, error) {
	formValue := r.FormValue(paramName)
	if formValue == "" {
		if paramName == startTimeParam {
			return p.timeNow().Add(-1 * p.traceQueryLookbackDuration), nil
		}
		return p.timeNow(), nil
	}
	t, err := strconv.ParseInt(formValue, 10, 64)
	if err != nil {
		return time.Time{}, newParseError(err, paramName)
	}
	return time.Unix(0, 0).Add(time.Duration(t) * units), nil
}

// parseDuration parses the duration parameter of an HTTP request that can be represented as either:
// - a duration string e.g.: "5ms"
// - a number of units of time e.g.: "1000"
// If the duration parameter is empty, the given defaultDuration will be returned.
func (p *queryParser) parseDuration(r *http.Request, paramName string, units *time.Duration, defaultDuration time.Duration) (d time.Duration, err error) {
	d = defaultDuration
	formValue := r.FormValue(paramName)
	switch {
	case formValue == "":
		return d, nil

	// If no units are supplied, assume parsing of duration strings like 5ms.
	case units == nil:
		if d, err = time.ParseDuration(formValue); err == nil {
			return d, nil
		}

	// Otherwise, the duration is a number for the given duration units.
	default:
		var i int64
		if i, err = strconv.ParseInt(formValue, 10, 64); err == nil {
			return time.Duration(i) * (*units), nil
		}
	}
	return 0, newParseError(err, paramName)
}

func (p *queryParser) parseBool(r *http.Request, paramName string) (b bool, err error) {
	formVal := r.FormValue(paramName)
	if formVal == "" {
		return false, nil
	}
	b, err = strconv.ParseBool(formVal)
	if err != nil {
		return b, newParseError(err, paramName)
	}
	return b, nil
}

// parseSpanKindParam parses the input comma-separated span kinds to filter for in the metrics query.
// Valid input span kinds are the string representations from the OpenTelemetry model/proto/metrics/otelspankind.proto.
// For example:
// - "SPAN_KIND_SERVER"
// - "SPAN_KIND_CLIENT"
// - etc.
func (p *queryParser) parseSpanKinds(r *http.Request, paramName string, defaultSpanKinds []string) ([]string, error) {
	formValue := r.FormValue(paramName)
	if formValue == "" {
		return defaultSpanKinds, nil
	}
	spanKinds := strings.Split(formValue, ",")
	if err := validateSpanKinds(spanKinds); err != nil {
		return defaultSpanKinds, newParseError(err, paramName)
	}
	return spanKinds, nil
}

func validateSpanKinds(spanKinds []string) error {
	for _, spanKind := range spanKinds {
		if _, ok := metrics.SpanKind_value[spanKind]; !ok {
			return fmt.Errorf("unsupported span kind: '%s'", spanKind)
		}
	}
	return nil
}

func (p *queryParser) validateQuery(traceQuery *traceQueryParameters) error {
	if len(traceQuery.traceIDs) == 0 && traceQuery.ServiceName == "" {
		return ErrServiceParameterRequired
	}
	if traceQuery.DurationMin != 0 && traceQuery.DurationMax != 0 {
		if traceQuery.DurationMax < traceQuery.DurationMin {
			return errMaxDurationGreaterThanMin
		}
	}
	return nil
}

func (p *queryParser) parseTags(simpleTags []string, jsonTags []string) (map[string]string, error) {
	retMe := make(map[string]string)
	for _, tag := range simpleTags {
		keyAndValue := strings.Split(tag, ":")
		if l := len(keyAndValue); l > 1 {
			retMe[keyAndValue[0]] = strings.Join(keyAndValue[1:], ":")
		} else {
			return nil, fmt.Errorf("malformed 'tag' parameter, expecting key:value, received: %s", tag)
		}
	}
	for _, tags := range jsonTags {
		var fromJSON map[string]string
		if err := json.Unmarshal([]byte(tags), &fromJSON); err != nil {
			return nil, fmt.Errorf("malformed 'tags' parameter, cannot unmarshal JSON: %s", err)
		}
		for k, v := range fromJSON {
			retMe[k] = v
		}
	}
	return retMe, nil
}

func newParseError(err error, paramName string) error {
	return fmt.Errorf("unable to parse param '%s': %w", paramName, err)
}
