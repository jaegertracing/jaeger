// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	defaultQueryLimit = 20

	operationParam   = "operation"
	tagParam         = "tag"
	startTimeParam   = "start"
	limitParam       = "limit"
	minDurationParam = "minDuration"
	maxDurationParam = "maxDuration"
	serviceParam     = "service"
	endTimeParam     = "end"
)

var (
	errCannotQueryTagAndDuration = fmt.Errorf("Cannot query for tags when '%s' is specified", minDurationParam)
	errMaxDurationGreaterThanMin = fmt.Errorf("'%s' should be greater than '%s'", maxDurationParam, minDurationParam)

	// ErrServiceParameterRequired occurs when no service name is defined
	ErrServiceParameterRequired = fmt.Errorf("Parameter '%s' is required", serviceParam)
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
//     param ::= service | operation | limit | start | end | minDuration | maxDuration | tag
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

	tags, err := p.parseTags(r.Form[tagParam])
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

	if minDuration != 0 && len(tags) > 0 {
		// This is because querying for this almost certainly returns no results due to intersections
		return nil, errCannotQueryTagAndDuration
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
			return nil, errors.Wrap(err, "cannot parse traceID param")
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
			return 0, errors.Wrapf(err, "Could not parse %s", durationParam)
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

func (p *queryParser) parseTags(tagsFromForm []string) (map[string]string, error) {
	retMe := make(map[string]string)
	for _, tag := range tagsFromForm {
		keyAndValue := strings.Split(tag, ":")
		if l := len(keyAndValue); l > 1 {
			retMe[keyAndValue[0]] = strings.Join(keyAndValue[1:], ":")
		} else {
			return nil, fmt.Errorf("Malformed 'tag' parameter, expecting key:value, received: %s", tag)
		}
	}
	return retMe, nil
}
