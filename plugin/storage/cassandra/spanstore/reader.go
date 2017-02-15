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
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
	"github.com/uber/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	querySpanByTraceID = `
		SELECT trace_id, span_id, parent_id, operation_name, flags, start_time, duration, tags, logs, refs, process 
		FROM traces 
		WHERE trace_id = ?`
)

type serviceNamesReader func() ([]string, error)

type operationNamesReader func(service string) ([]string, error)

// SpanReader can query for and load traces from Cassandra.
type SpanReader struct {
	session                   cassandra.Session
	consistency               cassandra.Consistency
	serviceNamesReader        serviceNamesReader
	operationNamesReader      operationNamesReader
	readTracesTableMetrics    *casMetrics.Table
	queryTraceTableMetrics    *casMetrics.Table
	queryTagIndexTableMetrics *casMetrics.Table
	logger                    zap.Logger
}

// NewSpanReader returns a new SpanReader.
func NewSpanReader(
	session cassandra.Session,
	metricsFactory metrics.Factory,
	logger zap.Logger,
) *SpanReader {
	readFactory := metricsFactory.Namespace("Read", nil)
	serviceNamesStorage := NewServiceNamesStorage(session, 0, metricsFactory, logger)
	operationNamesStorage := NewOperationNamesStorage(session, 0, metricsFactory, logger)
	return &SpanReader{
		session:                   session,
		consistency:               cassandra.One,
		serviceNamesReader:        serviceNamesStorage.GetServices,
		operationNamesReader:      operationNamesStorage.GetOperations,
		readTracesTableMetrics:    casMetrics.NewTable(readFactory, "ReadTraces"),
		queryTraceTableMetrics:    casMetrics.NewTable(readFactory, "QueryTraces"),
		queryTagIndexTableMetrics: casMetrics.NewTable(readFactory, "TagIndex"),
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
		err = errors.Wrap(err, "Error reading traces from storage")
		return nil, err
	}
	return retMe, nil
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	return s.readTrace(dbmodel.TraceIDFromDomain(traceID))
}

// FindTraces queries for traces.
func (s *SpanReader) FindTraces(queryParams *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	queries, err := BuildQueries(queryParams)
	if err != nil {
		return nil, err
	}
	mainQueryTraceIDs, err := s.executeQuery(queries.MainQuery, s.queryTraceTableMetrics)
	if err != nil {
		return nil, err
	}
	allTraceIDsFromQueries := make([]dbmodel.UniqueTraceIDs, 0, len(queries.TagQueries)+1)
	allTraceIDsFromQueries = append(allTraceIDsFromQueries, mainQueryTraceIDs)
	for _, tagQueryStmt := range queries.TagQueries {
		tagTraceIds, err := s.executeQuery(tagQueryStmt, s.queryTagIndexTableMetrics)
		if err != nil {
			return nil, err
		}
		allTraceIDsFromQueries = append(allTraceIDsFromQueries, tagTraceIds)
	}
	traceIDs := dbmodel.IntersectTraceIDs(allTraceIDsFromQueries)
	var retMe []*model.Trace
	for traceID := range traceIDs {
		if len(retMe) >= queries.NumTraces {
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

// executeQuery returns UniqueTraceIDs
func (s *SpanReader) executeQuery(query Query, tableMetrics *casMetrics.Table) (dbmodel.UniqueTraceIDs, error) {
	start := time.Now()
	q := s.session.Query(query.QueryString()).Bind(query.Parameters...)
	i := q.Consistency(s.consistency).Iter()
	var spans []dbmodel.Span
	var traceID dbmodel.TraceID
	var spanID int64
	for i.Scan(&traceID, &spanID) {
		traceAndSpanID := dbmodel.Span{
			TraceID: traceID,
			SpanID:  spanID,
		}
		spans = append(spans, traceAndSpanID)
	}
	err := i.Close()
	tableMetrics.Emit(err, time.Since(start))
	if err != nil {
		s.logger.Error("Failed to exec query", zap.String("query", query.QueryString()), zap.Object("params", query.Parameters), zap.Error(err))
		return nil, err
	}
	return dbmodel.GetUniqueTraceIDs(spans), nil
}
