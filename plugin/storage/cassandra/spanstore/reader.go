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

	queryByTag                     = `SELECT trace_id FROM tag_index WHERE service_name = ? AND tag_key = ? AND tag_value = ? and start_time > ? and start_time < ? limit ?`
	queryByServiceName             = `select trace_id from service_name_index where bucket in ` + bucketRange + ` and service_name = ? and start_time > ? and start_time < ? limit ?`
	queryByServiceAndOperationName = `select trace_id from service_operation_index where service_name = ? and operation_name = ? and start_time > ? and start_time < ? limit ?`
	queryByDuration                = `select trace_id from span_duration_index where bucket = ? and service_name = ? and span_name = ? and duration > ? and duration < ? limit ?`

	defaultNumTraces = 20
)

var (
	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("Malformed request object")
)

type serviceNamesReader func() ([]string, error)

type operationNamesReader func(service string) ([]string, error)

// SpanReader can query for and load traces from Cassandra.
type SpanReader struct {
	session                           cassandra.Session
	consistency                       cassandra.Consistency
	serviceNamesReader                serviceNamesReader
	operationNamesReader              operationNamesReader
	readTracesTableMetrics            *casMetrics.Table
	queryTraceTableMetrics            *casMetrics.Table
	queryTagIndexTableMetrics         *casMetrics.Table
	queryDurationIndexTableMetrics    *casMetrics.Table
	serviceOperationIndexTableMetrics *casMetrics.Table
	serviceNameIndexTableMetrics      *casMetrics.Table
	logger                            *zap.Logger
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
		session:                           session,
		consistency:                       cassandra.One,
		serviceNamesReader:                serviceNamesStorage.GetServices,
		operationNamesReader:              operationNamesStorage.GetOperations,
		readTracesTableMetrics:            casMetrics.NewTable(readFactory, "ReadTraces"),
		queryTraceTableMetrics:            casMetrics.NewTable(readFactory, "QueryTraces"),
		queryTagIndexTableMetrics:         casMetrics.NewTable(readFactory, "TagIndex"),
		queryDurationIndexTableMetrics:    casMetrics.NewTable(readFactory, "DurationIndex"),
		serviceOperationIndexTableMetrics: casMetrics.NewTable(readFactory, "ServiceOperationIndex"),
		serviceNameIndexTableMetrics:      casMetrics.NewTable(readFactory, "ServiceNameIndex"),
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
			s.readTracesTableMetrics.Emit(err, time.Since(start))
			return nil, err
		}
		retMe.Spans = append(retMe.Spans, span)
	}

	err := i.Close()
	s.readTracesTableMetrics.Emit(err, time.Since(start))
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

// FindTraces queries for traces.
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	if traceQuery == nil {
		return nil, ErrMalformedRequestObject
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}
	traceIDs, err := s.getTraceIDs(traceQuery)
	if err != nil {
		return nil, err
	}
	uniqueTraceIDs := map[dbmodel.TraceID]struct{}{}
	for _, traceID := range traceIDs {
		uniqueTraceIDs[traceID] = struct{}{}
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

// FindTraces queries for traces.
func (s *SpanReader) getTraceIDs(traceQuery *spanstore.TraceQueryParameters) ([]dbmodel.TraceID, error) {
	if traceQuery.DurationMin != 0 {
		return s.queryTraceIdsByDuration(traceQuery)
	}

	// NB(rooz): consider refactoring the query... methods where they take in traceQuery instead of a breakdown of its values
	if traceQuery.OperationName != "" {
		traceIds, err := s.queryByServiceNameAndOperation(traceQuery.ServiceName, traceQuery.OperationName, traceQuery.StartTimeMin, traceQuery.StartTimeMax, traceQuery.NumTraces)
		if err != nil {
			return nil, err
		}
		if len(traceQuery.Tags) > 0 {
			tagTraceIds, err := s.queryByTagsAndLogs(traceQuery.ServiceName, traceQuery.StartTimeMin, traceQuery.StartTimeMax, traceQuery.Tags, traceQuery.NumTraces)
			if err != nil {
				return nil, err
			}
			return s.intersectTraceIDSlices(traceIds, tagTraceIds), nil
		}
		return traceIds, err
	}
	if len(traceQuery.Tags) > 0 {
		return s.queryByTagsAndLogs(traceQuery.ServiceName, traceQuery.StartTimeMin, traceQuery.StartTimeMax, traceQuery.Tags, traceQuery.NumTraces)
	}
	return s.queryTraceIdsByService(traceQuery.ServiceName, traceQuery.StartTimeMin, traceQuery.StartTimeMax, traceQuery.NumTraces)
}

func (s *SpanReader) intersectTraceIDSlices(one, other []dbmodel.TraceID) []dbmodel.TraceID {
	oneAsMap := dbmodel.UniqueTraceIDs{}
	for _, i := range one {
		oneAsMap[i] = struct{}{}
	}
	var retMe []dbmodel.TraceID
	for _, i := range other {
		if _, ok := oneAsMap[i]; ok {
			retMe = append(retMe, i)
		}
	}
	return retMe
}

type traceIDResult struct {
	traceIDs []dbmodel.TraceID
	err      error
}

func (s *SpanReader) queryByTagsAndLogs(serviceName string, startTime, endTime time.Time, tags map[string]string, limit int) ([]dbmodel.TraceID, error) {
	var results []traceIDResult
	for k, v := range tags {
		// service_name = ? AND tag_key = ? AND tag_value = ? and start_time > ? and start_time < ? limit ?`
		query := s.session.Query(
			queryByTag,
			serviceName,
			k,
			v,
			model.TimeAsEpochMicroseconds(startTime),
			model.TimeAsEpochMicroseconds(endTime),
			limit,
		)
		t, err := s.executeQuery(query, s.queryTagIndexTableMetrics)
		results = append(results, traceIDResult{err: err, traceIDs: t})
	}
	unionedTraceIDs := map[dbmodel.TraceID]bool{}
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		for _, traceID := range result.traceIDs {
			unionedTraceIDs[traceID] = true
		}
	}
	var retMe []dbmodel.TraceID
	for k := range unionedTraceIDs {
		retMe = append(retMe, k)
	}
	return retMe, nil
}

func (s *SpanReader) queryTraceIdsByDuration(traceQuery *spanstore.TraceQueryParameters) ([]dbmodel.TraceID, error) {
	var traceIds []dbmodel.TraceID

	minDurationMicros := (traceQuery.DurationMin.Nanoseconds() / int64(time.Microsecond/time.Nanosecond))
	maxDurationMicros := (traceQuery.DurationMax.Nanoseconds() / int64(time.Microsecond/time.Nanosecond))

	// See writer.go:indexSpanDuration  for how this is indexed
	// This is indexed in hours since epoch, converted to seconds
	// TODO encapsulate this calculation
	startTimeSeconds := (traceQuery.StartTimeMin.UnixNano() / int64(time.Hour))
	endTimeSeconds := (traceQuery.StartTimeMax.UnixNano() / int64(time.Hour))

	for timeBucket := startTimeSeconds; timeBucket <= endTimeSeconds; timeBucket++ {
		query := s.session.Query(
			queryByDuration,
			timeBucket,
			traceQuery.ServiceName,
			traceQuery.OperationName,
			minDurationMicros,
			maxDurationMicros,
			traceQuery.NumTraces)
		t, err := s.executeQuery(query, s.queryDurationIndexTableMetrics)
		if err != nil {
			return nil, err
		}

		traceIds = append(traceIds, t...)
		if len(traceIds) > traceQuery.NumTraces {
			traceIds = traceIds[:traceQuery.NumTraces]
			break
		}
	}
	return traceIds, nil
}

func (s *SpanReader) queryByServiceNameAndOperation(serviceName string, operation string, startTime, endTime time.Time, limit int) ([]dbmodel.TraceID, error) {
	query := s.session.Query(queryByServiceAndOperationName, serviceName, operation, model.TimeAsEpochMicroseconds(startTime), model.TimeAsEpochMicroseconds(endTime), limit)
	return s.executeQuery(query, s.serviceOperationIndexTableMetrics)
}

func (s *SpanReader) queryTraceIdsByService(serviceName string, startTime, endTime time.Time, limit int) ([]dbmodel.TraceID, error) {
	query := s.session.Query(queryByServiceName, serviceName, model.TimeAsEpochMicroseconds(startTime), model.TimeAsEpochMicroseconds(endTime), limit)
	return s.executeQuery(query, s.serviceNameIndexTableMetrics)
}

// executeQuery returns UniqueTraceIDs
func (s *SpanReader) executeQuery(query cassandra.Query, tableMetrics *casMetrics.Table) ([]dbmodel.TraceID, error) {
	start := time.Now()
	i := query.Consistency(s.consistency).Iter()
	traceIDs := map[dbmodel.TraceID]struct{}{}
	var traceID dbmodel.TraceID
	for i.Scan(&traceID) {
		traceIDs[traceID] = struct{}{}
	}
	err := i.Close()
	tableMetrics.Emit(err, time.Since(start))
	if err != nil {
		s.logger.Error("Failed to exec query", zap.Error(err))
		return nil, err
	}
	var retMe []dbmodel.TraceID
	for traceID := range traceIDs {
		retMe = append(retMe, traceID)
	}
	return retMe, nil
}
