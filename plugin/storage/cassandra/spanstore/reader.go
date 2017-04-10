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
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
	"github.com/uber/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	bucketRange        = `(0,1,2,3,4,5,6,7,8,9)`
	querySpanByTraceID = `
		SELECT trace_id, span_id, parent_id, operation_name, flags, start_time, duration, tags, logs, refs, process
		FROM traces
		WHERE trace_id = ?`
	queryByTag = `
		SELECT trace_id
		FROM tag_index
		WHERE service_name = ? AND tag_key = ? AND tag_value = ? and start_time > ? and start_time < ?
		ORDER BY start_time DESC
		LIMIT ?`
	queryByServiceName = `
		SELECT trace_id
		FROM service_name_index
		WHERE bucket IN ` + bucketRange + ` AND service_name = ? AND start_time > ? AND start_time < ?
		ORDER BY start_time DESC
		LIMIT ?`
	queryByServiceAndOperationName = `
		SELECT trace_id
		FROM service_operation_index
		WHERE service_name = ? AND operation_name = ? AND start_time > ? AND start_time < ?
		ORDER BY start_time DESC
		LIMIT ?`
	queryByDuration = `
		SELECT trace_id
		FROM duration_index
		WHERE bucket = ? AND service_name = ? AND operation_name = ? AND duration > ? AND duration < ?
		LIMIT ?`

	defaultNumTraces = 100
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

	// ErrDurationAndTagQueryNotSupported occurs when duration and tags are both set
	ErrDurationAndTagQueryNotSupported = errors.New("Cannot query for duration and tags simultaneously")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("Start and End Time must be set")
)

type serviceNamesReader func() ([]string, error)

type operationNamesReader func(service string) ([]string, error)

type spanReaderMetrics struct {
	readTraces                 *casMetrics.Table
	queryTrace                 *casMetrics.Table
	queryTagIndex              *casMetrics.Table
	queryDurationIndex         *casMetrics.Table
	queryServiceOperationIndex *casMetrics.Table
	queryServiceNameIndex      *casMetrics.Table
}

// SpanReader can query for and load traces from Cassandra.
type SpanReader struct {
	session              cassandra.Session
	consistency          cassandra.Consistency
	serviceNamesReader   serviceNamesReader
	operationNamesReader operationNamesReader
	metrics              spanReaderMetrics
	logger               *zap.Logger
}

// NewSpanReader returns a new SpanReader.
func NewSpanReader(
	session cassandra.Session,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *SpanReader {
	readFactory := metricsFactory.Namespace("Read", nil)
	serviceNamesStorage := NewServiceNamesStorage(session, 0, metricsFactory, logger)
	operationNamesStorage := NewOperationNamesStorage(session, 0, metricsFactory, logger)
	return &SpanReader{
		session:              session,
		consistency:          cassandra.One,
		serviceNamesReader:   serviceNamesStorage.GetServices,
		operationNamesReader: operationNamesStorage.GetOperations,
		metrics: spanReaderMetrics{
			readTraces:                 casMetrics.NewTable(readFactory, "ReadTraces"),
			queryTrace:                 casMetrics.NewTable(readFactory, "QueryTraces"),
			queryTagIndex:              casMetrics.NewTable(readFactory, "TagIndex"),
			queryDurationIndex:         casMetrics.NewTable(readFactory, "DurationIndex"),
			queryServiceOperationIndex: casMetrics.NewTable(readFactory, "ServiceOperationIndex"),
			queryServiceNameIndex:      casMetrics.NewTable(readFactory, "ServiceNameIndex"),
		},
		logger: logger,
	}
}

// GetServices returns all services traced by Jaeger
func (s *SpanReader) GetServices() ([]string, error) {
	return s.serviceNamesReader()

}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
	return s.operationNamesReader(service)
}

func (s *SpanReader) readTrace(traceID dbmodel.TraceID) (*model.Trace, error) {
	start := time.Now()
	q := s.session.Query(querySpanByTraceID, traceID)
	i := q.Consistency(s.consistency).Iter()
	var traceIDFromSpan dbmodel.TraceID
	var startTime, spanID, duration, parentID int64
	var flags int32
	var operationName string
	var dbProcess dbmodel.Process
	var refs []dbmodel.SpanRef
	var tags []dbmodel.KeyValue
	var logs []dbmodel.Log
	retMe := &model.Trace{}
	for i.Scan(&traceIDFromSpan, &spanID, &parentID, &operationName, &flags, &startTime, &duration, &tags, &logs, &refs, &dbProcess) {
		dbSpan := dbmodel.Span{
			TraceID:       traceIDFromSpan,
			SpanID:        spanID,
			ParentID:      parentID,
			OperationName: operationName,
			Flags:         flags,
			StartTime:     startTime,
			Duration:      duration,
			Tags:          tags,
			Logs:          logs,
			Refs:          refs,
			Process:       dbProcess,
			ServiceName:   dbProcess.ServiceName,
		}
		span, err := dbmodel.ToDomain(&dbSpan)
		if err != nil {
			//do we consider conversion failure to cause such metrics to be emitted? for now i'm assuming yes.
			s.metrics.readTraces.Emit(err, time.Since(start))
			return nil, err
		}
		retMe.Spans = append(retMe.Spans, span)
	}

	err := i.Close()
	s.metrics.readTraces.Emit(err, time.Since(start))
	if err != nil {
		return nil, errors.Wrap(err, "Error reading traces from storage")
	}
	if len(retMe.Spans) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return retMe, nil
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	return s.readTrace(dbmodel.TraceIDFromDomain(traceID))
}

func validateQuery(p *spanstore.TraceQueryParameters) error {
	if p == nil {
		return ErrMalformedRequestObject
	}
	if p.ServiceName == "" && len(p.Tags) > 0 {
		return ErrServiceNameNotSet
	}
	if p.StartTimeMin.IsZero() || p.StartTimeMax.IsZero() {
		return ErrStartAndEndTimeNotSet
	}
	if !p.StartTimeMin.IsZero() && !p.StartTimeMax.IsZero() && p.StartTimeMax.Before(p.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return ErrDurationMinGreaterThanMax
	}
	if (p.DurationMin != 0 || p.DurationMax != 0) && len(p.Tags) > 0 {
		return ErrDurationAndTagQueryNotSupported
	}
	return nil
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}
	uniqueTraceIDs, err := s.findTraceIDs(traceQuery)
	if err != nil {
		return nil, err
	}
	var retMe []*model.Trace
	for traceID := range uniqueTraceIDs {
		if len(retMe) >= traceQuery.NumTraces {
			break
		}
		jTrace, err := s.readTrace(traceID)
		if err != nil {
			s.logger.Error("Failure to read trace", zap.String("trace_id", traceID.String()), zap.Error(err))
			continue
		}
		retMe = append(retMe, jTrace)
	}
	return retMe, nil
}

func (s *SpanReader) findTraceIDs(traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	if traceQuery.DurationMin != 0 || traceQuery.DurationMax != 0 {
		return s.queryByDuration(traceQuery)
	}

	if traceQuery.OperationName != "" {
		traceIds, err := s.queryByServiceNameAndOperation(traceQuery)
		if err != nil {
			return nil, err
		}
		if len(traceQuery.Tags) > 0 {
			tagTraceIds, err := s.queryByTagsAndLogs(traceQuery)
			if err != nil {
				return nil, err
			}
			return dbmodel.IntersectTraceIDs([]dbmodel.UniqueTraceIDs{
				traceIds,
				tagTraceIds,
			}), nil
		}
		return traceIds, nil
	}
	if len(traceQuery.Tags) > 0 {
		return s.queryByTagsAndLogs(traceQuery)
	}
	return s.queryByService(traceQuery)
}

func (s *SpanReader) queryByTagsAndLogs(tq *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	results := make([]dbmodel.UniqueTraceIDs, 0, len(tq.Tags))
	for k, v := range tq.Tags {
		query := s.session.Query(
			queryByTag,
			tq.ServiceName,
			k,
			v,
			model.TimeAsEpochMicroseconds(tq.StartTimeMin),
			model.TimeAsEpochMicroseconds(tq.StartTimeMax),
			tq.NumTraces,
		)
		t, err := s.executeQuery(query, s.metrics.queryTagIndex)
		if err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return dbmodel.IntersectTraceIDs(results), nil
}

func (s *SpanReader) queryByDuration(traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	results := dbmodel.UniqueTraceIDs{}

	minDurationMicros := traceQuery.DurationMin.Nanoseconds() / int64(time.Microsecond/time.Nanosecond)
	maxDurationMicros := (time.Hour * 24).Nanoseconds() / int64(time.Microsecond/time.Nanosecond)
	if traceQuery.DurationMax != 0 {
		maxDurationMicros = traceQuery.DurationMax.Nanoseconds() / int64(time.Microsecond/time.Nanosecond)
	}

	// See writer.go:indexByDuration  for how this is indexed
	// This is indexed in hours since epoch
	startTimeByHour := traceQuery.StartTimeMin.Round(durationBucketSize)
	endTimeByHour := traceQuery.StartTimeMax.Round(durationBucketSize)

	for timeBucket := endTimeByHour; timeBucket.After(startTimeByHour) || timeBucket.Equal(startTimeByHour); timeBucket = timeBucket.Add(-1 * durationBucketSize) {
		query := s.session.Query(
			queryByDuration,
			timeBucket,
			traceQuery.ServiceName,
			traceQuery.OperationName,
			minDurationMicros,
			maxDurationMicros,
			traceQuery.NumTraces)
		t, err := s.executeQuery(query, s.metrics.queryDurationIndex)
		if err != nil {
			return nil, err
		}

		for traceID := range t {
			results.Add(traceID)
			if len(results) == traceQuery.NumTraces {
				break
			}
		}
	}
	return results, nil
}

func (s *SpanReader) queryByServiceNameAndOperation(tq *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	query := s.session.Query(queryByServiceAndOperationName, tq.ServiceName, tq.OperationName, model.TimeAsEpochMicroseconds(tq.StartTimeMin), model.TimeAsEpochMicroseconds(tq.StartTimeMax), tq.NumTraces)
	return s.executeQuery(query, s.metrics.queryServiceOperationIndex)
}

func (s *SpanReader) queryByService(tq *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	query := s.session.Query(queryByServiceName, tq.ServiceName, model.TimeAsEpochMicroseconds(tq.StartTimeMin), model.TimeAsEpochMicroseconds(tq.StartTimeMax), tq.NumTraces)
	return s.executeQuery(query, s.metrics.queryServiceNameIndex)
}

func (s *SpanReader) executeQuery(query cassandra.Query, tableMetrics *casMetrics.Table) (dbmodel.UniqueTraceIDs, error) {
	start := time.Now()
	i := query.Consistency(s.consistency).Iter()
	retMe := dbmodel.UniqueTraceIDs{}
	var traceID dbmodel.TraceID
	for i.Scan(&traceID) {
		retMe.Add(traceID)
	}
	err := i.Close()
	tableMetrics.Emit(err, time.Since(start))
	if err != nil {
		s.logger.Error("Failed to exec query", zap.Error(err))
		return nil, err
	}
	return retMe, nil
}
