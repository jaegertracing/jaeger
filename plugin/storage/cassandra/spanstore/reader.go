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

package spanstore

import (
	"context"
	"time"

	"github.com/opentracing/opentracing-go"
	ottag "github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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
	// limitMultiple exists because many spans that are returned from indices can have the same trace, limitMultiple increases
	// the number of responses from the index, so we can respect the user's limit value they provided.
	limitMultiple = 3
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
	readFactory := metricsFactory.Namespace(metrics.NSOptions{Name: "read", Tags: nil})
	serviceNamesStorage := NewServiceNamesStorage(session, 0, metricsFactory, logger)
	operationNamesStorage := NewOperationNamesStorage(session, 0, metricsFactory, logger)
	return &SpanReader{
		session:              session,
		serviceNamesReader:   serviceNamesStorage.GetServices,
		operationNamesReader: operationNamesStorage.GetOperations,
		metrics: spanReaderMetrics{
			readTraces:                 casMetrics.NewTable(readFactory, "read_traces"),
			queryTrace:                 casMetrics.NewTable(readFactory, "query_traces"),
			queryTagIndex:              casMetrics.NewTable(readFactory, "tag_index"),
			queryDurationIndex:         casMetrics.NewTable(readFactory, "duration_index"),
			queryServiceOperationIndex: casMetrics.NewTable(readFactory, "service_operation_index"),
			queryServiceNameIndex:      casMetrics.NewTable(readFactory, "service_name_index"),
		},
		logger: logger,
	}
}

// GetServices returns all services traced by Jaeger
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	return s.serviceNamesReader()

}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(ctx context.Context, service string) ([]string, error) {
	return s.operationNamesReader(service)
}

func (s *SpanReader) readTrace(ctx context.Context, traceID dbmodel.TraceID) (*model.Trace, error) {
	span, ctx := startSpanForQuery(ctx, "readTrace", querySpanByTraceID)
	defer span.Finish()
	span.LogFields(otlog.String("event", "searching"), otlog.Object("trace_id", traceID))

	trace, err := s.readTraceInSpan(ctx, traceID)
	logErrorToSpan(span, err)
	return trace, err
}

func (s *SpanReader) readTraceInSpan(ctx context.Context, traceID dbmodel.TraceID) (*model.Trace, error) {
	start := time.Now()
	q := s.session.Query(querySpanByTraceID, traceID)
	i := q.Iter()
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
func (s *SpanReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return s.readTrace(ctx, dbmodel.TraceIDFromDomain(traceID))
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
func (s *SpanReader) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	uniqueTraceIDs, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	var retMe []*model.Trace
	for _, traceID := range uniqueTraceIDs {
		jTrace, err := s.GetTrace(ctx, traceID)
		if err != nil {
			s.logger.Error("Failure to read trace", zap.String("trace_id", traceID.String()), zap.Error(err))
			continue
		}
		retMe = append(retMe, jTrace)
	}
	return retMe, nil
}

// FindTraceIDs retrieve traceIDs that match the traceQuery
func (s *SpanReader) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}

	dbTraceIDs, err := s.findTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}

	var traceIDs []model.TraceID
	for t := range dbTraceIDs {
		if len(traceIDs) >= traceQuery.NumTraces {
			break
		}
		traceIDs = append(traceIDs, t.ToDomain())
	}
	return traceIDs, nil
}

func (s *SpanReader) findTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	if traceQuery.DurationMin != 0 || traceQuery.DurationMax != 0 {
		return s.queryByDuration(ctx, traceQuery)
	}

	if traceQuery.OperationName != "" {
		traceIds, err := s.queryByServiceNameAndOperation(ctx, traceQuery)
		if err != nil {
			return nil, err
		}
		if len(traceQuery.Tags) > 0 {
			tagTraceIds, err := s.queryByTagsAndLogs(ctx, traceQuery)
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
		return s.queryByTagsAndLogs(ctx, traceQuery)
	}
	return s.queryByService(ctx, traceQuery)
}

func (s *SpanReader) queryByTagsAndLogs(ctx context.Context, tq *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	span, ctx := startSpanForQuery(ctx, "queryByTagsAndLogs", queryByTag)
	defer span.Finish()

	results := make([]dbmodel.UniqueTraceIDs, 0, len(tq.Tags))
	for k, v := range tq.Tags {
		childSpan, _ := opentracing.StartSpanFromContext(ctx, "queryByTag")
		childSpan.LogFields(otlog.String("tag.key", k), otlog.String("tag.value", v))
		query := s.session.Query(
			queryByTag,
			tq.ServiceName,
			k,
			v,
			model.TimeAsEpochMicroseconds(tq.StartTimeMin),
			model.TimeAsEpochMicroseconds(tq.StartTimeMax),
			tq.NumTraces*limitMultiple,
		).PageSize(0)
		t, err := s.executeQuery(childSpan, query, s.metrics.queryTagIndex)
		childSpan.Finish()
		if err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return dbmodel.IntersectTraceIDs(results), nil
}

func (s *SpanReader) queryByDuration(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	span, ctx := startSpanForQuery(ctx, "queryByDuration", queryByDuration)
	defer span.Finish()

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
		childSpan, _ := opentracing.StartSpanFromContext(ctx, "queryForTimeBucket")
		childSpan.LogFields(otlog.String("timeBucket", timeBucket.String()))
		query := s.session.Query(
			queryByDuration,
			timeBucket,
			traceQuery.ServiceName,
			traceQuery.OperationName,
			minDurationMicros,
			maxDurationMicros,
			traceQuery.NumTraces*limitMultiple)
		t, err := s.executeQuery(childSpan, query, s.metrics.queryDurationIndex)
		childSpan.Finish()
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

func (s *SpanReader) queryByServiceNameAndOperation(ctx context.Context, tq *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	//lint:ignore SA4006 failing to re-assign context is worse than unused variable
	span, ctx := startSpanForQuery(ctx, "queryByServiceNameAndOperation", queryByServiceAndOperationName)
	defer span.Finish()
	query := s.session.Query(
		queryByServiceAndOperationName,
		tq.ServiceName,
		tq.OperationName,
		model.TimeAsEpochMicroseconds(tq.StartTimeMin),
		model.TimeAsEpochMicroseconds(tq.StartTimeMax),
		tq.NumTraces*limitMultiple,
	).PageSize(0)
	return s.executeQuery(span, query, s.metrics.queryServiceOperationIndex)
}

func (s *SpanReader) queryByService(ctx context.Context, tq *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	//lint:ignore SA4006 failing to re-assign context is worse than unused variable
	span, ctx := startSpanForQuery(ctx, "queryByService", queryByServiceName)
	defer span.Finish()
	query := s.session.Query(
		queryByServiceName,
		tq.ServiceName,
		model.TimeAsEpochMicroseconds(tq.StartTimeMin),
		model.TimeAsEpochMicroseconds(tq.StartTimeMax),
		tq.NumTraces*limitMultiple,
	).PageSize(0)
	return s.executeQuery(span, query, s.metrics.queryServiceNameIndex)
}

func (s *SpanReader) executeQuery(span opentracing.Span, query cassandra.Query, tableMetrics *casMetrics.Table) (dbmodel.UniqueTraceIDs, error) {
	start := time.Now()
	i := query.Iter()
	retMe := dbmodel.UniqueTraceIDs{}
	var traceID dbmodel.TraceID
	for i.Scan(&traceID) {
		retMe.Add(traceID)
	}
	err := i.Close()
	tableMetrics.Emit(err, time.Since(start))
	if err != nil {
		logErrorToSpan(span, err)
		span.LogFields(otlog.String("query", query.String()))
		s.logger.Error("Failed to exec query", zap.Error(err), zap.String("query", query.String()))
		return nil, err
	}
	return retMe, nil
}

func startSpanForQuery(ctx context.Context, name, query string) (opentracing.Span, context.Context) {
	span, ctx := opentracing.StartSpanFromContext(ctx, name)
	ottag.DBStatement.Set(span, query)
	ottag.DBType.Set(span, "cassandra")
	ottag.Component.Set(span, "gocql")
	return span, ctx
}

func logErrorToSpan(span opentracing.Span, err error) {
	if err == nil {
		return
	}
	ottag.Error.Set(span, true)
	span.LogFields(otlog.Error(err))
}
