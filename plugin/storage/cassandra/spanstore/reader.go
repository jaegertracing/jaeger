// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
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
	ErrServiceNameNotSet = errors.New("service Name must be set")

	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("start Time Minimum is above Maximum")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("duration Minimum is above Maximum")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("malformed request object")

	// ErrDurationAndTagQueryNotSupported occurs when duration and tags are both set
	ErrDurationAndTagQueryNotSupported = errors.New("cannot query for duration and tags simultaneously")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("start and End Time must be set")
)

type serviceNamesReader func() ([]string, error)

type operationNamesReader func(query spanstore.OperationQueryParameters) ([]spanstore.Operation, error)

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
	tracer               trace.Tracer
}

// NewSpanReader returns a new SpanReader.
func NewSpanReader(
	session cassandra.Session,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	tracer trace.Tracer,
) (*SpanReader, error) {
	readFactory := metricsFactory.Namespace(metrics.NSOptions{Name: "read", Tags: nil})
	serviceNamesStorage := NewServiceNamesStorage(session, 0, metricsFactory, logger)
	operationNamesStorage, err := NewOperationNamesStorage(session, 0, metricsFactory, logger)
	if err != nil {
		return nil, err
	}
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
		tracer: tracer,
	}, nil
}

// GetServices returns all services traced by Jaeger
func (s *SpanReader) GetServices(context.Context) ([]string, error) {
	return s.serviceNamesReader()
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(
	_ context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return s.operationNamesReader(query)
}

func (s *SpanReader) readTrace(ctx context.Context, traceID dbmodel.TraceID) (*model.Trace, error) {
	ctx, span := s.startSpanForQuery(ctx, "readTrace", querySpanByTraceID)
	defer span.End()
	span.SetAttributes(attribute.Key("trace_id").String(traceID.String()))

	trace, err := s.readTraceInSpan(ctx, traceID)
	logErrorToSpan(span, err)
	return trace, err
}

func (s *SpanReader) readTraceInSpan(_ context.Context, traceID dbmodel.TraceID) (*model.Trace, error) {
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
		return nil, fmt.Errorf("error reading traces from storage: %w", err)
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
	ctx, span := s.startSpanForQuery(ctx, "queryByTagsAndLogs", queryByTag)
	defer span.End()

	results := make([]dbmodel.UniqueTraceIDs, 0, len(tq.Tags))
	for k, v := range tq.Tags {
		_, childSpan := s.tracer.Start(ctx, "queryByTag")
		childSpan.SetAttributes(
			attribute.Key("tag.key").String(k),
			attribute.Key("tag.value").String(v),
		)
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
		childSpan.End()
		if err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return dbmodel.IntersectTraceIDs(results), nil
}

func (s *SpanReader) queryByDuration(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	ctx, span := s.startSpanForQuery(ctx, "queryByDuration", queryByDuration)
	defer span.End()

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
		_, childSpan := s.tracer.Start(ctx, "queryForTimeBucket")
		childSpan.SetAttributes(attribute.Key("timeBucket").String(timeBucket.String()))
		query := s.session.Query(
			queryByDuration,
			timeBucket,
			traceQuery.ServiceName,
			traceQuery.OperationName,
			minDurationMicros,
			maxDurationMicros,
			traceQuery.NumTraces*limitMultiple)
		t, err := s.executeQuery(childSpan, query, s.metrics.queryDurationIndex)
		childSpan.End()
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
	_, span := s.startSpanForQuery(ctx, "queryByServiceNameAndOperation", queryByServiceAndOperationName)
	defer span.End()
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
	_, span := s.startSpanForQuery(ctx, "queryByService", queryByServiceAndOperationName)
	defer span.End()
	query := s.session.Query(
		queryByServiceName,
		tq.ServiceName,
		model.TimeAsEpochMicroseconds(tq.StartTimeMin),
		model.TimeAsEpochMicroseconds(tq.StartTimeMax),
		tq.NumTraces*limitMultiple,
	).PageSize(0)
	return s.executeQuery(span, query, s.metrics.queryServiceNameIndex)
}

func (s *SpanReader) executeQuery(span trace.Span, query cassandra.Query, tableMetrics *casMetrics.Table) (dbmodel.UniqueTraceIDs, error) {
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
		s.logger.Error("Failed to exec query", zap.Error(err), zap.String("query", query.String()))
		return nil, err
	}
	return retMe, nil
}

func (s *SpanReader) startSpanForQuery(ctx context.Context, name, query string) (context.Context, trace.Span) {
	ctx, span := s.tracer.Start(ctx, name)
	span.SetAttributes(
		attribute.Key(otelsemconv.DBQueryTextKey).String(query),
		attribute.Key(otelsemconv.DBSystemKey).String("cassandra"),
		attribute.Key("component").String("gocql"),
	)
	return ctx, span
}

func logErrorToSpan(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
