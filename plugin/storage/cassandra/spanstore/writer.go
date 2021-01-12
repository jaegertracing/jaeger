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
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

const (
	insertSpan = `
		INSERT
		INTO traces(trace_id, span_id, span_hash, parent_id, operation_name, flags,
				    start_time, duration, tags, logs, refs, process)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	serviceNameIndex = `
		INSERT
		INTO service_name_index(service_name, bucket, start_time, trace_id)
		VALUES (?, ?, ?, ?)`

	serviceOperationIndex = `
		INSERT
		INTO
		service_operation_index(service_name, operation_name, start_time, trace_id)
		VALUES (?, ?, ?, ?)`

	tagIndex = `
		INSERT
		INTO tag_index(trace_id, span_id, service_name, start_time, tag_key, tag_value)
		VALUES (?, ?, ?, ?, ?, ?)`

	durationIndex = `
		INSERT
		INTO duration_index(service_name, operation_name, bucket, duration, start_time, trace_id)
		VALUES (?, ?, ?, ?, ?, ?)`

	maximumTagKeyOrValueSize = 256

	// DefaultNumBuckets Number of buckets for bucketed keys
	defaultNumBuckets = 10

	durationBucketSize = time.Hour
)

const (
	storeFlag = storageMode(1 << iota)
	indexFlag
)

type storageMode uint8
type serviceNamesWriter func(serviceName string) error
type operationNamesWriter func(operation dbmodel.Operation) error

type spanWriterMetrics struct {
	traces                *casMetrics.Table
	tagIndex              *casMetrics.Table
	serviceNameIndex      *casMetrics.Table
	serviceOperationIndex *casMetrics.Table
	durationIndex         *casMetrics.Table
}

// SpanWriter handles all writes to Cassandra for the Jaeger data model
type SpanWriter struct {
	session              cassandra.Session
	serviceNamesWriter   serviceNamesWriter
	operationNamesWriter operationNamesWriter
	writerMetrics        spanWriterMetrics
	logger               *zap.Logger
	tagIndexSkipped      metrics.Counter
	tagFilter            dbmodel.TagFilter
	storageMode          storageMode
	indexFilter          dbmodel.IndexFilter
}

// NewSpanWriter returns a SpanWriter
func NewSpanWriter(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	options ...Option,
) *SpanWriter {
	serviceNamesStorage := NewServiceNamesStorage(session, writeCacheTTL, metricsFactory, logger)
	operationNamesStorage := NewOperationNamesStorage(session, writeCacheTTL, metricsFactory, logger)
	tagIndexSkipped := metricsFactory.Counter(metrics.Options{Name: "tag_index_skipped", Tags: nil})
	opts := applyOptions(options...)
	return &SpanWriter{
		session:              session,
		serviceNamesWriter:   serviceNamesStorage.Write,
		operationNamesWriter: operationNamesStorage.Write,
		writerMetrics: spanWriterMetrics{
			traces:                casMetrics.NewTable(metricsFactory, "traces"),
			tagIndex:              casMetrics.NewTable(metricsFactory, "tag_index"),
			serviceNameIndex:      casMetrics.NewTable(metricsFactory, "service_name_index"),
			serviceOperationIndex: casMetrics.NewTable(metricsFactory, "service_operation_index"),
			durationIndex:         casMetrics.NewTable(metricsFactory, "duration_index"),
		},
		logger:          logger,
		tagIndexSkipped: tagIndexSkipped,
		tagFilter:       opts.tagFilter,
		storageMode:     opts.storageMode,
		indexFilter:     opts.indexFilter,
	}
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
	s.session.Close()
	return nil
}

// WriteSpan saves the span into Cassandra
func (s *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	ds := dbmodel.FromDomain(span)
	if s.storageMode&storeFlag == storeFlag {
		if err := s.writeSpan(span, ds); err != nil {
			return err
		}
	}
	if s.storageMode&indexFlag == indexFlag {
		if err := s.writeIndexes(span, ds); err != nil {
			return err
		}
	}
	return nil
}

func (s *SpanWriter) writeSpan(span *model.Span, ds *dbmodel.Span) error {
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
	if err := s.writerMetrics.traces.Exec(mainQuery, s.logger); err != nil {
		return s.logError(ds, err, "Failed to insert span", s.logger)
	}
	return nil
}

func (s *SpanWriter) writeIndexes(span *model.Span, ds *dbmodel.Span) error {
	spanKind, _ := span.GetSpanKind()
	if err := s.saveServiceNameAndOperationName(dbmodel.Operation{
		ServiceName:   ds.ServiceName,
		SpanKind:      spanKind,
		OperationName: ds.OperationName,
	}); err != nil {
		// should this be a soft failure?
		return s.logError(ds, err, "Failed to insert service name and operation name", s.logger)
	}

	if s.indexFilter(ds, dbmodel.ServiceIndex) {
		if err := s.indexByService(ds); err != nil {
			return s.logError(ds, err, "Failed to index service name", s.logger)
		}
	}

	if s.indexFilter(ds, dbmodel.OperationIndex) {
		if err := s.indexByOperation(ds); err != nil {
			return s.logError(ds, err, "Failed to index operation name", s.logger)
		}
	}

	if span.Flags.IsFirehoseEnabled() {
		return nil // skipping expensive indexing
	}

	if err := s.indexByTags(span, ds); err != nil {
		return s.logError(ds, err, "Failed to index tags", s.logger)
	}

	if s.indexFilter(ds, dbmodel.DurationIndex) {
		if err := s.indexByDuration(ds, span.StartTime); err != nil {
			return s.logError(ds, err, "Failed to index duration", s.logger)
		}
	}
	return nil
}

func (s *SpanWriter) indexByTags(span *model.Span, ds *dbmodel.Span) error {
	for _, v := range dbmodel.GetAllUniqueTags(span, s.tagFilter) {
		// we should introduce retries or just ignore failures imo, retrying each individual tag insertion might be better
		// we should consider bucketing.
		if s.shouldIndexTag(v) {
			insertTagQuery := s.session.Query(tagIndex, ds.TraceID, ds.SpanID, v.ServiceName, ds.StartTime, v.TagKey, v.TagValue)
			if err := s.writerMetrics.tagIndex.Exec(insertTagQuery, s.logger); err != nil {
				withTagInfo := s.logger.
					With(zap.String("tag_key", v.TagKey)).
					With(zap.String("tag_value", v.TagValue)).
					With(zap.String("service_name", v.ServiceName))
				return s.logError(ds, err, "Failed to index tag", withTagInfo)
			}
		} else {
			s.tagIndexSkipped.Inc(1)
		}
	}
	return nil
}

func (s *SpanWriter) indexByDuration(span *dbmodel.Span, startTime time.Time) error {
	query := s.session.Query(durationIndex)
	timeBucket := startTime.Round(durationBucketSize)
	var err error
	indexByOperationName := func(operationName string) {
		q1 := query.Bind(span.Process.ServiceName, operationName, timeBucket, span.Duration, span.StartTime, span.TraceID)
		if err2 := s.writerMetrics.durationIndex.Exec(q1, s.logger); err2 != nil {
			s.logError(span, err2, "Cannot index duration", s.logger)
			err = err2
		}
	}
	indexByOperationName("")                 // index by service name alone
	indexByOperationName(span.OperationName) // index by service name and operation name
	return err
}

func (s *SpanWriter) indexByService(span *dbmodel.Span) error {
	bucketNo := uint64(span.SpanHash) % defaultNumBuckets
	query := s.session.Query(serviceNameIndex)
	q := query.Bind(span.Process.ServiceName, bucketNo, span.StartTime, span.TraceID)
	return s.writerMetrics.serviceNameIndex.Exec(q, s.logger)
}

func (s *SpanWriter) indexByOperation(span *dbmodel.Span) error {
	query := s.session.Query(serviceOperationIndex)
	q := query.Bind(span.Process.ServiceName, span.OperationName, span.StartTime, span.TraceID)
	return s.writerMetrics.serviceOperationIndex.Exec(q, s.logger)
}

// shouldIndexTag checks to see if the tag is json or not, if it's UTF8 valid and it's not too large
func (s *SpanWriter) shouldIndexTag(tag dbmodel.TagInsertion) bool {
	isJSON := func(s string) bool {
		var js json.RawMessage
		// poor man's string-is-a-json check shortcircuits full unmarshalling
		return strings.HasPrefix(s, "{") && json.Unmarshal([]byte(s), &js) == nil
	}

	return len(tag.TagKey) < maximumTagKeyOrValueSize &&
		len(tag.TagValue) < maximumTagKeyOrValueSize &&
		utf8.ValidString(tag.TagValue) &&
		utf8.ValidString(tag.TagKey) &&
		!isJSON(tag.TagValue)
}

func (s *SpanWriter) logError(span *dbmodel.Span, err error, msg string, logger *zap.Logger) error {
	logger.
		With(zap.String("trace_id", span.TraceID.String())).
		With(zap.Int64("span_id", span.SpanID)).
		With(zap.Error(err)).
		Error(msg)
	return fmt.Errorf("%s: %w", msg, err)
}

func (s *SpanWriter) saveServiceNameAndOperationName(operation dbmodel.Operation) error {
	if err := s.serviceNamesWriter(operation.ServiceName); err != nil {
		return err
	}
	return s.operationNamesWriter(operation)
}
