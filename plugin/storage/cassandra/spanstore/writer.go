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
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
	"github.com/uber/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

const (
	insertSpan = `
		INSERT 
		INTO traces(trace_id, span_id, span_hash, parent_id, operation_name, flags, 
				    start_time, duration, tags, logs, refs, process, service_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	insertTag = `INSERT INTO tag_index(trace_id, span_id, service_name, tag_key, tag_value) VALUES (?, ?, ?, ?, ?)`

	maximumTagKeyOrValueSize = 256
)

type serviceNamesWriter func(serviceName string) error
type operationNamesWriter func(serviceName, operationName string) error

// SpanWriter handles all writes to Cassandra for the Jaeger data model
type SpanWriter struct {
	session              cassandra.Session
	serviceNamesWriter   serviceNamesWriter
	operationNamesWriter operationNamesWriter
	tracesTableMetrics   *casMetrics.Table
	tagIndexTableMetrics *casMetrics.Table
	logger               zap.Logger
	tagIndexSkipped      metrics.Counter
}

// NewSpanWriter returns a SpanWriter
func NewSpanWriter(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger zap.Logger,
) *SpanWriter {
	serviceNamesStorage := NewServiceNamesStorage(session, writeCacheTTL, metricsFactory, logger)
	operationNamesStorage := NewOperationNamesStorage(session, writeCacheTTL, metricsFactory, logger)
	tagIndexSkipped := metricsFactory.Counter("tagIndexSkipped", nil)
	return &SpanWriter{
		session:              session,
		serviceNamesWriter:   serviceNamesStorage.Write,
		operationNamesWriter: operationNamesStorage.Write,
		tracesTableMetrics:   casMetrics.NewTable(metricsFactory, "Traces"),
		tagIndexTableMetrics: casMetrics.NewTable(metricsFactory, "TagIndex"),
		logger:               logger,
		tagIndexSkipped:      tagIndexSkipped,
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
		ds.ServiceName,
	)
	err := s.tracesTableMetrics.Exec(mainQuery, s.logger)
	if err != nil {
		return s.logError(ds, err, "Failed to insert span", s.logger)
	}
	err = s.saveServiceNameAndOperationName(ds.ServiceName, ds.OperationName)
	if err != nil {
		// should this be a soft failure?
		return s.logError(ds, err, "Failed to insert service name and operation name", s.logger)
	}

	for _, v := range dbmodel.GetAllUniqueTags(span) {
		// we should introduce retries or just ignore failures imo, retrying each individual tag insertion might be better
		// we should consider bucketing.
		if s.shouldIndexTag(v) {
			insertTagQuery := s.session.Query(insertTag, ds.TraceID, ds.SpanID, v.ServiceName, v.TagKey, v.TagValue)
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
	return nil
}

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

func (s *SpanWriter) logError(span *dbmodel.Span, err error, msg string, logger zap.Logger) error {
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
