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

package spanstore

import (
	"bytes"
	"errors"
	"time"

	"github.com/uber/jaeger/storage/spanstore"
)

const (
	baseQuery          = `SELECT trace_id, span_id FROM traces`
	serviceNameClause  = `service_name = ?`
	operationClause    = `operation_name = ?`
	startTimeMinClause = `start_time >= ?`
	startTimeMaxClause = `start_time <= ?`
	durationMinClause  = `duration >= ?`
	durationMaxClause  = `duration <= ?`
	allowFiltering     = ` ALLOW FILTERING`

	tagQuery = `SELECT trace_id, span_id FROM tag_index WHERE service_name = ? AND tag_key = ? AND tag_value = ?`
)

var (
	// ErrServiceNameNotSet occurs when attempting to query with an empty service name
	ErrServiceNameNotSet = errors.New("Service Name must be set")

	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("Start Time Minimum is above Maximum")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("Duration Minimum is above Maximum")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("Malformed request object")

	dawnOfTime = time.Unix(0, 0)
)

// Query encapsulates a query string with placeholders and parameters to bind to those placeholders
type Query struct {
	buffer         bytes.Buffer
	Parameters     []interface{}
	hasBaseClause  bool
	hasWhereClause bool
}

// QueryString returns the final query string.
func (q *Query) QueryString() string {
	return q.buffer.String()
}

// append query adds some portion of a query string to a Query
//
// It is stateful and works in a state-machine fashion. The first call to append() just adds
// a string and parameters, the second call adds " WHERE " plus a clause, and
// futher calls add " AND " plus another clause.
//
// append should be used to add anything to a Query to avoid breaking the state machine
func (q *Query) append(queryString string, parameters ...interface{}) {
	if !q.hasBaseClause {
		q.hasBaseClause = true
	} else if !q.hasWhereClause {
		q.buffer.WriteString(" WHERE ")
		q.hasWhereClause = true
	} else {
		q.buffer.WriteString(" AND ")
	}

	q.buffer.WriteString(queryString)
	q.Parameters = append(q.Parameters, parameters...)
}

func (q *Query) appendNotEmpty(queryString string, value string) {
	if value != "" {
		q.append(queryString, value)
	}
}

func (q *Query) appendTimeIfNotZero(queryString string, value time.Time) {
	if !value.IsZero() {
		microseconds := value.Sub(dawnOfTime).Nanoseconds() / 1000
		q.append(queryString, microseconds)
	}
}

func (q *Query) appendDurationIfNotZero(queryString string, value time.Duration) {
	if value != 0 {
		microseconds := uint64(value / 1000)
		q.append(queryString, microseconds)
	}
}

// Queries is a construct that provides the main query against cassandra and any optional tag queries
type Queries struct {
	spanstore.TraceQueryParameters
	MainQuery  Query
	TagQueries []Query
}

// BuildQueries takes a query request and generates Queries
func BuildQueries(p *spanstore.TraceQueryParameters) (*Queries, error) {
	if p == nil {
		return nil, ErrMalformedRequestObject
	}
	if p.ServiceName == "" && len(p.Tags) > 0 {
		return nil, ErrServiceNameNotSet
	}
	if !p.StartTimeMin.IsZero() && !p.StartTimeMax.IsZero() && p.StartTimeMax.Before(p.StartTimeMin) {
		return nil, ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return nil, ErrDurationMinGreaterThanMax
	}
	q := &Queries{TraceQueryParameters: *p}
	q.buildTagQueries()
	q.buildMainQuery()
	return q, nil
}

func (q *Queries) buildMainQuery() {
	q.MainQuery.append(baseQuery)
	q.MainQuery.appendNotEmpty(serviceNameClause, q.ServiceName)
	q.MainQuery.appendNotEmpty(operationClause, q.OperationName)
	q.MainQuery.appendTimeIfNotZero(startTimeMinClause, q.StartTimeMin)
	q.MainQuery.appendTimeIfNotZero(startTimeMaxClause, q.StartTimeMax)
	q.MainQuery.appendDurationIfNotZero(durationMinClause, q.DurationMin)
	q.MainQuery.appendDurationIfNotZero(durationMaxClause, q.DurationMax)

	// The one exception to the append() pattern, since ALLOW FILTERING doesn't use an AND prefix
	q.MainQuery.buffer.WriteString(allowFiltering)
}

func (q *Queries) buildTagQueries() {
	if len(q.Tags) == 0 {
		return
	}
	q.TagQueries = make([]Query, 0, len(q.Tags))
	for k, v := range q.Tags {
		qq := Query{}
		qq.append(tagQuery, q.ServiceName, k, v)
		q.TagQueries = append(q.TagQueries, qq)
	}
}
