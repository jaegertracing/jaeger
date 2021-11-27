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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/model"
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

	// ErrServiceParameterRequired occurs when no service name is defined
	ErrServiceParameterRequired = fmt.Errorf("parameter '%s' is required", serviceParam)
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

// parse takes a request and constructs a model of parameters
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
func (p *queryParser) parse(r *http.Request) (*traceQueryParameters, error) {
	service := r.FormValue(serviceParam)
	operation := r.FormValue(operationParam)

	startTime, err := p.parseTime(startTimeParam, r)
	if err != nil {
		return nil, err
	}
	endTime, err := p.parseTime(endTimeParam, r)
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

	minDuration, err := p.parseDuration(minDurationParam, r)
	if err != nil {
		return nil, err
	}

	maxDuration, err := p.parseDuration(maxDurationParam, r)
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

func (p *queryParser) parseTime(param string, r *http.Request) (time.Time, error) {
	value := r.FormValue(param)
	if value == "" {
		if param == startTimeParam {
			return p.timeNow().Add(-1 * p.traceQueryLookbackDuration), nil
		}
		return p.timeNow(), nil
	}
	micros, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, 0).Add(time.Duration(micros) * time.Microsecond), nil
}

func (p *queryParser) parseDuration(durationParam string, r *http.Request) (time.Duration, error) {
	durationInput := r.FormValue(durationParam)
	if len(durationInput) > 0 {
		duration, err := time.ParseDuration(durationInput)
		if err != nil {
			return 0, fmt.Errorf("cannot not parse %s: %w", durationParam, err)
		}
		return duration, nil
	}
	return 0, nil
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
