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
	spanKindParam    = "spanKind"
	endTimeParam     = "end"
	prettyPrintParam = "prettyPrint"
)

var (
	errMaxDurationGreaterThanMin = fmt.Errorf("'%s' should be greater than '%s'", maxDurationParam, minDurationParam)

	// errServiceParameterRequired occurs when no service name is defined.
	errServiceParameterRequired = fmt.Errorf("parameter '%s' is required", serviceParam)

	jaegerToOtelSpanKind = map[string]string{
		"unspecified": metrics.SpanKind_SPAN_KIND_UNSPECIFIED.String(),
		"internal":    metrics.SpanKind_SPAN_KIND_INTERNAL.String(),
		"server":      metrics.SpanKind_SPAN_KIND_SERVER.String(),
		"client":      metrics.SpanKind_SPAN_KIND_CLIENT.String(),
		"producer":    metrics.SpanKind_SPAN_KIND_PRODUCER.String(),
		"consumer":    metrics.SpanKind_SPAN_KIND_CONSUMER.String(),
	}
)

type (
	// queryParser handles the parsing of query parameters for traces.
	queryParser struct {
		traceQueryLookbackDuration time.Duration
		timeNow                    func() time.Time
	}

	traceQueryParameters struct {
		spanstore.TraceQueryParameters
		traceIDs []model.TraceID
	}

	dependenciesQueryParameters struct {
		endTs    time.Time
		lookback time.Duration
	}

	durationParser = func(s string) (time.Duration, error)
)

func newDurationStringParser() durationParser {
	return func(s string) (time.Duration, error) {
		return time.ParseDuration(s)
	}
}

func newDurationUnitsParser(units time.Duration) durationParser {
	return func(s string) (time.Duration, error) {
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(i) * (units), nil
	}
}

// parseTraceQueryParams takes a request and constructs a model of parameters.
//
// Why start/end parameters are expressed in microseconds:
// Span searches operate on span latencies, which are expressed as microseconds in the data model, hence why
// support for high accuracy in search query parameters is required.
// Microsecond precision is a legacy artifact from zipkin origins where timestamps and durations
// are in microseconds (see: https://zipkin.io/pages/instrumenting.html).
//
// Why duration parameters are expressed as duration strings like "1ms":
// The search UI itself does not insist on exact units because it supports string like 1ms.
// Go makes parsing duration strings like "1ms" very easy, hence why parsing of such strings is
// deferred to the backend rather than Jaeger UI.
//
// Trace query syntax:
//
//	query ::= param | param '&' query
//	param ::= service | operation | limit | start | end | minDuration | maxDuration | tag | tags
//	service ::= 'service=' strValue
//	operation ::= 'operation=' strValue
//	limit ::= 'limit=' intValue
//	start ::= 'start=' intValue in unix microseconds
//	end ::= 'end=' intValue in unix microseconds
//	minDuration ::= 'minDuration=' strValue (units are "ns", "us" (or "µs"), "ms", "s", "m", "h")
//	maxDuration ::= 'maxDuration=' strValue (units are "ns", "us" (or "µs"), "ms", "s", "m", "h")
//	tag ::= 'tag=' key | 'tag=' keyvalue
//	key := strValue
//	keyValue := strValue ':' strValue
//	tags :== 'tags=' jsonMap
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

	parser := newDurationStringParser()
	minDuration, err := parseDuration(r, minDurationParam, parser, 0)
	if err != nil {
		return nil, err
	}

	maxDuration, err := parseDuration(r, maxDurationParam, parser, 0)
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

// parseDependenciesQueryParams takes a request and constructs a model of dependencies query parameters.
//
// The dependencies API does not operate on the latency space, instead its timestamps are just time range selections,
// and the typical backend granularity of those is on the order of 15min or more. As such, microseconds aren't
// useful in this domain and milliseconds are sufficient for both times and durations.
func (p *queryParser) parseDependenciesQueryParams(r *http.Request) (dqp dependenciesQueryParameters, err error) {
	dqp.endTs, err = p.parseTime(r, endTsParam, time.Millisecond)
	if err != nil {
		return dqp, err
	}

	dqp.lookback, err = parseDuration(r, lookbackParam, newDurationUnitsParser(time.Millisecond), defaultDependencyLookbackDuration)
	return dqp, err
}

// parseMetricsQueryParams takes a request and constructs a model of metrics query parameters.
//
// Why the API is designed using an end time (endTs) and lookback:
// The typical usage of the metrics APIs is to view the most recent metrics from now looking
// back a certain period of time, given the value of metrics generally degrades with time. As such, the API
// is also designed to mirror the user interface inputs.
//
// Why times are expressed as unix milliseconds:
//   - The minimum step size for Prometheus-compliant metrics backends is 1ms,
//     hence millisecond precision on times is sufficient.
//   - The metrics API is designed with one primary client in mind, the Jaeger UI. As it is a React.js application,
//     the maximum supported built-in time precision is milliseconds, making it a convenient precision to use for such a client.
//
// Why durations are expressed as unix milliseconds:
//   - Given the endTs time is expressed as milliseconds, it follows that lookback durations should use the
//     same time units to compute the start time.
//   - As above, the minimum step size for Prometheus-compliant metrics backends is 1ms.
//   - Other durations are in milliseconds to maintain consistency of units with other parameters in the metrics APIs.
//   - As the primary client for the metrics API is the Jaeger UI, it is programmatically simpler to supply the
//     integer representations of durations in milliseconds rather than the human-readable representation such as "1ms".
//
// Metrics query syntax:
//
//	query ::= services , [ '&' optionalParams ]
//	optionalParams := param | param '&' optionalParams
//	param ::=  groupByOperation | endTs | lookback | step | ratePer | spanKinds
//	services ::= service | service '&' services
//	service ::= 'service=' strValue
//	groupByOperation ::= 'groupByOperation=' boolValue
//	endTs ::= 'endTs=' intValue in unix milliseconds
//	lookback ::= 'lookback=' intValue duration in milliseconds
//	step ::= 'step=' intValue duration in milliseconds
//	ratePer ::= 'ratePer=' intValue duration in milliseconds
//	spanKinds ::= spanKind | spanKind '&' spanKinds
//	spanKind ::= 'spanKind=' spanKindType
//	spanKindType ::= "unspecified" | "internal" | "server" | "client" | "producer" | "consumer"
func (p *queryParser) parseMetricsQueryParams(r *http.Request) (bqp metricsstore.BaseQueryParameters, err error) {
	query := r.URL.Query()
	services, ok := query[serviceParam]
	if !ok {
		return bqp, newParseError(errors.New("please provide at least one service name"), serviceParam)
	}
	bqp.ServiceNames = services

	bqp.GroupByOperation, err = parseBool(r, groupByOperationParam)
	if err != nil {
		return bqp, err
	}
	bqp.SpanKinds, err = parseSpanKinds(r, spanKindParam, defaultMetricsSpanKinds)
	if err != nil {
		return bqp, err
	}
	endTs, err := p.parseTime(r, endTsParam, time.Millisecond)
	if err != nil {
		return bqp, err
	}
	parser := newDurationUnitsParser(time.Millisecond)
	lookback, err := parseDuration(r, lookbackParam, parser, defaultMetricsQueryLookbackDuration)
	if err != nil {
		return bqp, err
	}
	step, err := parseDuration(r, stepParam, parser, defaultMetricsQueryStepDuration)
	if err != nil {
		return bqp, err
	}
	ratePer, err := parseDuration(r, rateParam, parser, defaultMetricsQueryRateDuration)
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

// parseDuration parses the duration parameter of an HTTP request using the provided durationParser.
// If the duration parameter is empty, the given defaultDuration will be returned.
func parseDuration(r *http.Request, paramName string, parse durationParser, defaultDuration time.Duration) (time.Duration, error) {
	formValue := r.FormValue(paramName)
	if formValue == "" {
		return defaultDuration, nil
	}
	d, err := parse(formValue)
	if err != nil {
		return 0, newParseError(err, paramName)
	}
	return d, nil
}

func parseBool(r *http.Request, paramName string) (b bool, err error) {
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

// parseSpanKindParam parses the input span kinds to filter for in the metrics query.
//
// Valid input span kinds include:
// - "unspecified": when no span kind specified in span.
// - "internal": internal operation within an application, instead of application boundaries.
// - "server": server-side handling span.
// - "client": outbound service call span.
// - "producer": producer sending a message to broker.
// - "consumer": consumer consuming a message from a broker.
//
// The output span kinds are the string representations from the OpenTelemetry model/proto/metrics/otelspankind.proto.
// That is, the following map to the above valid inputs:
// - "SPAN_KIND_UNSPECIFIED"
// - "SPAN_KIND_INTERNAL"
// - "SPAN_KIND_SERVER"
// - "SPAN_KIND_CLIENT"
// - "SPAN_KIND_PRODUCER"
// - "SPAN_KIND_CONSUMER"
func parseSpanKinds(r *http.Request, paramName string, defaultSpanKinds []string) ([]string, error) {
	query := r.URL.Query()
	jaegerSpanKinds, ok := query[paramName]
	if !ok {
		return defaultSpanKinds, nil
	}
	otelSpanKinds, err := mapSpanKindsToOpenTelemetry(jaegerSpanKinds)
	if err != nil {
		return defaultSpanKinds, newParseError(err, paramName)
	}
	return otelSpanKinds, nil
}

func mapSpanKindsToOpenTelemetry(spanKinds []string) ([]string, error) {
	otelSpanKinds := make([]string, len(spanKinds))
	for i, spanKind := range spanKinds {
		if v, ok := jaegerToOtelSpanKind[spanKind]; ok {
			otelSpanKinds[i] = v
		} else {
			return otelSpanKinds, fmt.Errorf("unsupported span kind: '%s'", spanKind)
		}
	}
	return otelSpanKinds, nil
}

func (p *queryParser) validateQuery(traceQuery *traceQueryParameters) error {
	if len(traceQuery.traceIDs) == 0 && traceQuery.ServiceName == "" {
		return errServiceParameterRequired
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
			return nil, fmt.Errorf("malformed 'tags' parameter, cannot unmarshal JSON: %w", err)
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
