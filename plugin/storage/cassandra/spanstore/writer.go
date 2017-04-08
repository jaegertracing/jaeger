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
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dgryski/go-farm"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
	"github.com/uber/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

const (
	insertSpan = `
		INSERT
		INTO traces(trace_id, span_id, span_hash, parent_id, operation_name, flags,
				    start_time, duration, tags, logs, refs, process)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	insertTag = `INSERT INTO tag_index(trace_id, span_id, service_name, start_time, tag_key, tag_value) VALUES (?, ?, ?, ?, ?, ?)`

	serviceNameIndex = `INSERT INTO service_name_index(service_name, bucket, start_time, trace_id) VALUES (?, ?, ?, ?)`

	serviceOperationIndex = `INSERT INTO service_operation_index(service_name, operation_name, start_time, trace_id) VALUES (?, ?, ?, ?)`

	durationIndex = `INSERT INTO duration_index(service_name, operation_name, bucket, duration, start_time, trace_id) VALUES (?, ?, ?, ?, ?, ?)`

	maximumTagKeyOrValueSize = 256

	// DefaultNumBuckets Number of buckets for bucketed keys
	defaultNumBuckets = 10
)

type serviceNamesWriter func(serviceName string) error
type operationNamesWriter func(serviceName, operationName string) error

// SpanWriter handles all writes to Cassandra for the Jaeger data model
type SpanWriter struct {
	session                 cassandra.Session
	serviceNamesWriter      serviceNamesWriter
	operationNamesWriter    operationNamesWriter
	tracesTableMetrics      *casMetrics.Table
	tagIndexTableMetrics    *casMetrics.Table
	serviceNameIndexMetrics *casMetrics.Table
	serviceOperationIndex   *casMetrics.Table
	durationIndex           *casMetrics.Table
	logger                  *zap.Logger
	tagIndexSkipped         metrics.Counter
}

// NewSpanWriter returns a SpanWriter
func NewSpanWriter(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *SpanWriter {
	serviceNamesStorage := NewServiceNamesStorage(session, writeCacheTTL, metricsFactory, logger)
	operationNamesStorage := NewOperationNamesStorage(session, writeCacheTTL, metricsFactory, logger)
	tagIndexSkipped := metricsFactory.Counter("tagIndexSkipped", nil)
	return &SpanWriter{
		session:                 session,
		serviceNamesWriter:      serviceNamesStorage.Write,
		operationNamesWriter:    operationNamesStorage.Write,
		tracesTableMetrics:      casMetrics.NewTable(metricsFactory, "Traces"),
		tagIndexTableMetrics:    casMetrics.NewTable(metricsFactory, "TagIndex"),
		serviceNameIndexMetrics: casMetrics.NewTable(metricsFactory, "ServiceNameIndex"),
		serviceOperationIndex:   casMetrics.NewTable(metricsFactory, "ServiceOperationIndex"),
		durationIndex:           casMetrics.NewTable(metricsFactory, "DurationIndex"),
		logger:                  logger,
		tagIndexSkipped:         tagIndexSkipped,
	}
}

// WriteSpan saves the span into Cassandra
func (s *SpanWriter) WriteSpan(span *model.Span) error {
	ds := dbmodel.FromDomain(span)
	mainQuery := s.session.Query(
		insertSpan,
		ds.TraceID,
		ds.SpanID,
		ds.SpanHash,
		ds.ParentID,
		ds.OperationName,
		ds.Flags,
		ds.StartTime,
		ds.Duration,
		ds.Tags,
		ds.Logs,
		ds.Refs,
		ds.Process,
	)

	if err := s.tracesTableMetrics.Exec(mainQuery, s.logger); err != nil {
		return s.logError(ds, err, "Failed to insert span", s.logger)
	}
	if err := s.saveServiceNameAndOperationName(ds.ServiceName, ds.OperationName); err != nil {
		// should this be a soft failure?
		return s.logError(ds, err, "Failed to insert service name and operation name", s.logger)
	}

	for _, v := range dbmodel.GetAllUniqueTags(span) {
		// we should introduce retries or just ignore failures imo, retrying each individual tag insertion might be better
		// we should consider bucketing.
		if s.shouldIndexTag(v) {
			insertTagQuery := s.session.Query(insertTag, ds.TraceID, ds.SpanID, v.ServiceName, ds.StartTime, v.TagKey, v.TagValue)
			if err := s.tagIndexTableMetrics.Exec(insertTagQuery, s.logger); err != nil {
				withTagInfo := s.logger.
					With(zap.String("tag_key", v.TagKey)).
					With(zap.String("tag_value", v.TagValue)).
					With(zap.String("service_name", v.ServiceName))
				return s.logError(ds, err, "Failed to insert tag", withTagInfo)
			}
		} else {
			s.tagIndexSkipped.Inc(1)
		}
	}

	if err := s.indexTraceIDByServiceAndOperation(span.TraceID, ds); err != nil {
		return s.logError(ds, err, "Failed to insert service and operation name into index", s.logger)
	}

	if err := s.indexSpanDuration(ds); err != nil {
		return s.logError(ds, err, "Failed to insert duration into index", s.logger)
	}
	return nil
}

func (s *SpanWriter) indexSpanDuration(span *dbmodel.Span) error {
	query := s.session.Query(durationIndex)
	timeBucket := int((span.StartTime / (60 * 60)) / 1000000) // 1hr in microseconds TODO use config
	var err error
	indexByOperationName := func(operationName string) {
		q1 := query.Bind(span.Process.ServiceName, operationName, timeBucket, span.Duration, span.StartTime, span.TraceID)
		if err2 := s.durationIndex.Exec(q1, s.logger); err2 != nil {
			err = err2
		}
	}
	indexByOperationName("")                 // index by service name alone
	indexByOperationName(span.OperationName) // index by service name and operation name
	return err
}

func (s *SpanWriter) indexTraceIDByServiceAndOperation(traceID model.TraceID, span *dbmodel.Span) error {
	startTime := span.StartTime
	bucketNo := traceIDBucket(traceID.Low, defaultNumBuckets)
	query1 := s.session.Query(serviceNameIndex)
	query2 := s.session.Query(serviceOperationIndex)
	var err error
	q1 := query1.Bind(span.Process.ServiceName, bucketNo, startTime, span.TraceID)
	err2 := s.serviceNameIndexMetrics.Exec(q1, s.logger)
	if err2 != nil {
		err = err2
	}
	q2 := query2.Bind(span.Process.ServiceName, span.OperationName, startTime, span.TraceID)
	err2 = s.serviceOperationIndex.Exec(q2, s.logger)
	if err2 != nil {
		err = err2
	}
	return err
}

func traceIDHash32(traceID uint64) uint32 {
	var buf [binary.MaxVarintLen64]byte
	sbuf := buf[:] // below methods take a slice
	binary.PutUvarint(sbuf, traceID)
	return farm.Hash32(sbuf)
}
func traceIDBucket(traceIDLow uint64, numBuckets uint32) uint32 {
	return traceIDHash32(traceIDLow) % numBuckets
}

// shouldIndexTag checks to see if the tag is json or not, if it's UTF8 valid and it's not too large
func (s *SpanWriter) shouldIndexTag(tag dbmodel.TagInsertion) bool {
	isJSON := func(s string) bool {
		var js map[string]interface{}
		// poor man's string-is-a-json check shortcircuits full unmarshalling
		return strings.HasPrefix(s, "{") && json.Unmarshal([]byte(s), &js) == nil
	}

	return len(tag.TagKey) < maximumTagKeyOrValueSize &&
		len(tag.TagValue) < maximumTagKeyOrValueSize &&
		utf8.ValidString(tag.TagValue) &&
		!isJSON(tag.TagValue)
}

func (s *SpanWriter) logError(span *dbmodel.Span, err error, msg string, logger *zap.Logger) error {
	logger.
		With(zap.String("trace_id", span.TraceID.String())).
		With(zap.Int64("span_id", span.SpanID)).
		With(zap.Error(err)).
		Error(msg)
	return errors.Wrap(err, msg)
}

func (s *SpanWriter) saveServiceNameAndOperationName(serviceName, operationName string) error {
	if err := s.serviceNamesWriter(serviceName); err != nil {
		return err
	}
	return s.operationNamesWriter(serviceName, operationName)
}
