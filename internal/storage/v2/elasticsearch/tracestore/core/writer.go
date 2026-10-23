// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// SpanWriter writes spans and their service:operation pairs through an
// esclient.BatchWriter — the async indexer or the synchronous bulk writer, chosen by
// the factory. Which one is not the writer's concern; it assembles the batch and
// hands it over the same way regardless.
type SpanWriter struct {
	batchWriter       esclient.BatchWriter
	serviceOp         *ServiceOperationStorage
	logger            *zap.Logger
	writerMetrics     *spanstoremetrics.WriteMetrics
	spanRotation      indices.Rotation
	serviceRotation   indices.Rotation
	allTagsAsFields   bool
	tagDotReplacement string
	tagKeysAsFields   map[string]bool
}

// Writer is a DB-Level abstraction which directly deals with database level operations
type Writer interface {
	// WriteSpans writes a batch of spans and their corresponding service:operation
	// pairs to Elasticsearch/OpenSearch through the configured batch writer,
	// returning its error. It is the batch entry point the v2 TraceWriter drives
	// once per WriteTraces call.
	WriteSpans(ctx context.Context, spans []dbmodel.Span) error
	// Close closes Writer
	Close() error
}

// SpanWriterParams holds constructor parameters for NewSpanWriter. BatchWriter is
// the destination for assembled documents; the factory supplies the async or
// synchronous implementation, so the writer stays mode-agnostic. ServiceOperations
// carries the service:operation dedup cache and is owned by the factory rather than
// the writer, so that Purge can clear it.
type SpanWriterParams struct {
	BatchWriter       esclient.BatchWriter
	Logger            *zap.Logger
	MetricsFactory    metrics.Factory
	ServiceOperations *ServiceOperationStorage
	AllTagsAsFields   bool
	TagKeysAsFields   []string
	TagDotReplacement string
	SpanRotation      indices.Rotation
	ServiceRotation   indices.Rotation
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(p SpanWriterParams) *SpanWriter {
	tags := map[string]bool{}
	for _, k := range p.TagKeysAsFields {
		tags[k] = true
	}

	return &SpanWriter{
		batchWriter:       p.BatchWriter,
		serviceOp:         p.ServiceOperations,
		logger:            p.Logger,
		writerMetrics:     spanstoremetrics.NewWriter(p.MetricsFactory, "spans"),
		spanRotation:      p.SpanRotation,
		serviceRotation:   p.ServiceRotation,
		tagKeysAsFields:   tags,
		allTagsAsFields:   p.AllTagsAsFields,
		tagDotReplacement: p.TagDotReplacement,
	}
}

// WriteSpans assembles the whole batch's documents — each span, plus any new
// service:operation dedup docs — and writes them through the batch writer, returning
// its error. The service cache is marked only after a successful write (§4.3), so
// a failed-and-retried batch re-sends the service docs rather than skipping them.
func (s *SpanWriter) WriteSpans(ctx context.Context, spans []dbmodel.Span) error {
	items := make([]esclient.BulkItem, 0, len(spans))
	serviceOps := newServiceOperationBatch(s.serviceOp)
	// itemIDToSpan maps each document's _id to the span behind it, nil for a
	// service:operation lookup document. It lets a per-document rejection reported
	// by the batch writer be attributed back to the span it concerns (RFC 0007 §4.8).
	itemIDToSpan := make(map[string]*dbmodel.Span, len(spans))
	for i := range spans {
		span := &spans[i]
		s.writerMetrics.Attempts.Inc(1)
		spanStartTime := model.EpochMicrosecondsAsTime(span.StartTime)

		// Service:operation pair doc, deduped to one doc per batch unless already cached.
		if item, ok := serviceOps.toUpsertItem(s.serviceRotation.WriteTarget(spanStartTime), span); ok {
			items = append(items, item)
			itemIDToSpan[item.ID] = nil
		}

		// Span doc.
		s.convertNestedTagsToFieldTags(span)
		if s.spanRotation.RequiresDocumentTimestamp() {
			span.Timestamp = spanStartTime.Format(time.RFC3339Nano)
		}
		item, err := s.buildSpanItem(s.spanRotation.WriteTarget(spanStartTime), span)
		if err != nil {
			// Drop a span that still cannot be encoded rather than fail the batch.
			// Non-finite floats are rewritten to JSON strings in buildSpanItem, so
			// they are not what lands here.
			s.writerMetrics.Errors.Inc(1)
			s.logger.Error("failed to encode span document", zap.Error(err))
			continue
		}
		items = append(items, item)
		itemIDToSpan[item.ID] = span
	}

	err := s.batchWriter.WriteBatch(ctx, items)
	if err != nil {
		err = s.attributeRejections(err, itemIDToSpan)
	}
	if batchSettled(err) {
		// The batch will not be written again: every document other than the
		// reported poison spans is durable (or enqueued, in async mode), so the
		// service docs are cached. That includes a service:operation document that
		// the backend rejected with a terminal error: sending it again would only
		// fail again.
		serviceOps.commitToCache()
	}
	return err
}

// batchSettled reports whether the batch will not be written again: it succeeded,
// or its only failures were terminal rejections attributed to spans, which the
// caller re-routes rather than retries. A transient failure or an unidentified
// document means a retry, and the service cache waits for it.
func batchSettled(err error) bool {
	if err == nil {
		return true
	}
	var rejected *tracestore.RejectedSpansError
	return errors.As(err, &rejected) && !rejected.Transient && rejected.Unidentified == 0
}

// attributeRejections turns a batch writer error that lists terminally-rejected
// documents into a tracestore.RejectedSpansError naming the spans concerned, so a
// caller can re-route them without knowing the document layout. Any other error is
// returned as is. A rejected service:operation lookup document is logged and not
// reported: the spans it indexes are stored, and the pair is cached like any other
// once the batch completes, so the document is not re-sent to be rejected again
// until the writer restarts. A batch whose only terminal rejections are lookup
// documents therefore succeeds. A rejected document whose _id is not in
// itemIDToSpan is counted as unidentified, because it could be a span. An error
// with no terminal rejections (a transport failure, or only transient rejections)
// is returned as is.
//
// Documents are matched by _id alone: the bulk response names the concrete backing
// index behind a write alias or data stream, not the index the item was sent to,
// and ids cannot collide across the two document kinds (a span's is
// traceID_spanID_hash, a lookup's a hash).
func (s *SpanWriter) attributeRejections(err error, itemIDToSpan map[string]*dbmodel.Span) error {
	var bulkErr *esclient.BulkWriteError
	if !errors.As(err, &bulkErr) || len(bulkErr.Terminal) == 0 {
		return err
	}
	rejected := &tracestore.RejectedSpansError{Transient: bulkErr.Transient, Err: err}
	for _, item := range bulkErr.Terminal {
		span, ok := itemIDToSpan[item.ID]
		if !ok {
			rejected.Unidentified++
			continue
		}
		if span != nil {
			traceID, terr := span.TraceID.ToOTEL()
			spanID, serr := span.SpanID.ToOTEL()
			if terr != nil || serr != nil {
				// Spans produced by ToDBModel carry valid hex ids, so this should not
				// happen; treat it as unidentified rather than lose the span.
				rejected.Unidentified++
				continue
			}
			rejected.Spans = append(rejected.Spans, tracestore.RejectedSpan{TraceID: traceID, SpanID: spanID, Reason: item.Reason})
			continue
		}
		s.logger.Warn("service:operation lookup document rejected by the backend; its spans are stored and the pair is not re-sent until restart",
			zap.String("id", item.ID), zap.Int("status", item.Status), zap.String("reason", item.Reason))
	}
	if len(rejected.Spans) == 0 && rejected.Unidentified == 0 {
		// The only terminal rejections were lookup documents. Every span is stored, so
		// the batch is complete unless transient failures still call for a retry.
		if rejected.Transient {
			return err
		}
		return nil
	}
	return rejected
}

func (s *SpanWriter) convertNestedTagsToFieldTags(span *dbmodel.Span) {
	processNestedTags, processFieldTags := s.splitElevatedTags(span.Process.Tags)
	span.Process.Tags = processNestedTags
	span.Process.Tag = processFieldTags
	nestedTags, fieldTags := s.splitElevatedTags(span.Tags)
	span.Tags = nestedTags
	span.Tag = fieldTags
}

// Close is a no-op: the writer owns no resources. The bulk indexer that backs it
// is owned and closed by the factory (which flushes buffered writes on shutdown).
func (*SpanWriter) Close() error {
	return nil
}

// buildSpanItem marshals a span into a BulkItem with a deterministic _id, returning
// an error if it cannot be encoded — the caller decides what to do. Non-finite
// float64 values are rewritten to JSON strings first so a NaN or ±Inf tag is stored
// instead of rejecting the document. The span is marshaled once here; the bytes are
// reused for the _id and handed to the batch writer as json.RawMessage so they are
// not re-encoded.
func (s *SpanWriter) buildSpanItem(indexName string, jsonSpan *dbmodel.Span) (esclient.BulkItem, error) {
	// Marshal a copy when a non-finite float has to be rewritten. The caller's
	// span keeps the original float64, including slices and maps it shares with
	// this value.
	body, err := json.Marshal(encodeNonFiniteFloats(jsonSpan))
	if err != nil {
		return esclient.BulkItem{}, err
	}
	return esclient.BulkItem{
		Index:  indexName,
		ID:     spanDocID(jsonSpan, body),
		OpType: s.spanRotation.WriteOpType(),
		Body:   json.RawMessage(body),
	}, nil
}

// spanDocID returns a deterministic _id for a span document, which makes span
// writes idempotent: re-sending an identical span reuses its _id, so the write
// upserts instead of creating a duplicate (on a data-stream create it is rejected
// as a benign 409). This matters because at-least-once ingest makes retries a
// routine event rather than a rare edge case (RFC 0007 §4.7).
//
// The id is the trace ID, the span ID, and a content hash of the document. The
// (traceID, spanID) prefix is not there for uniqueness — the hash already covers
// those fields — but to bound the hash's job. Two documents can collide only if they
// share (traceID, spanID), so a collision can never merge spans from different
// traces, and the hash only has to separate the ≤2 spans that legitimately share
// those IDs: a client and a server span in the Zipkin shared-span model (which is
// also why the discriminator must be a content hash, not a positional key like
// startTime). Disambiguating so few documents is trivial, so a 64-bit hash is ample
// and its width is not load-bearing — unlike a bare-hash id, whose collision
// resistance would have to hold across every document in the index. (This is the
// same uniqueness as Cassandra's (trace_id, span_id, span_hash); there the composite
// key also drives the read path, which does not apply to an ES _id.)
//
// The three parts are joined with "_", which never occurs in a hex trace/span ID or
// hash, so the id is injective by construction: distinct (traceID, spanID) always
// map to distinct ids. A delimiter rather than fixed-width concatenation matters
// because an ID is not reliably fixed-width — TraceID/SpanID render empty for a zero
// id — and the whole point of the prefix is a structural no-cross-trace-collision
// guarantee, not one resting on a width assumption with edge cases.
//
// body is the span's marshaled JSON, passed in so the caller marshals only once.
// The hash is stable across retries because the conversion is deterministic: the
// span's ordered slices (Tags, Logs, References) come from the OTLP attributes in
// stored order (pcommon.Map is slice-backed, not a Go map, so iteration is ordered),
// and encoding/json emits struct fields in definition order with sorted map keys, so
// a byte-identical span always yields the same body and id.
func spanDocID(span *dbmodel.Span, body []byte) string {
	h := fnv.New64a()
	h.Write(body)
	return string(span.TraceID) + "_" + string(span.SpanID) + "_" + strconv.FormatUint(h.Sum64(), 16)
}

// encodeNonFiniteFloats returns the span to marshal. NaN and ±Inf become the
// OTLP/JSON strings "NaN", "Infinity", and "-Infinity" so encoding/json accepts
// the document, and KeyValue.Type stays float64. The input is returned as-is
// when every float is finite. Otherwise only the slices and maps that hold a
// non-finite value are copied, and the copy is rewritten.
func encodeNonFiniteFloats(span *dbmodel.Span) *dbmodel.Span {
	tags, tagsChanged := rewriteKeyValues(span.Tags)
	logs, logsChanged := rewriteLogs(span.Logs)
	tag, tagChanged := rewriteTagMap(span.Tag)
	procTags, procTagsChanged := rewriteKeyValues(span.Process.Tags)
	procTag, procTagChanged := rewriteTagMap(span.Process.Tag)
	if !tagsChanged && !logsChanged && !tagChanged && !procTagsChanged && !procTagChanged {
		return span
	}
	clone := *span
	if tagsChanged {
		clone.Tags = tags
	}
	if logsChanged {
		clone.Logs = logs
	}
	if tagChanged {
		clone.Tag = tag
	}
	if procTagsChanged {
		clone.Process.Tags = procTags
	}
	if procTagChanged {
		clone.Process.Tag = procTag
	}
	return &clone
}

// rewriteFloat returns the JSON-safe form of a non-finite float64 and true.
// Finite floats and every other type are left alone.
func rewriteFloat(v any) (any, bool) {
	f, ok := v.(float64)
	if !ok || (!math.IsNaN(f) && !math.IsInf(f, 0)) {
		return nil, false
	}
	return dbmodel.JSONSafeFloat(f), true
}

func rewriteKeyValues(kvs []dbmodel.KeyValue) ([]dbmodel.KeyValue, bool) {
	var cloned []dbmodel.KeyValue
	for i := range kvs {
		safe, changed := rewriteFloat(kvs[i].Value)
		if !changed {
			continue
		}
		if cloned == nil {
			cloned = append([]dbmodel.KeyValue(nil), kvs...)
		}
		cloned[i].Value = safe
	}
	if cloned == nil {
		return kvs, false
	}
	return cloned, true
}

func rewriteLogs(logs []dbmodel.Log) ([]dbmodel.Log, bool) {
	var cloned []dbmodel.Log
	for i := range logs {
		fields, changed := rewriteKeyValues(logs[i].Fields)
		if !changed {
			continue
		}
		if cloned == nil {
			cloned = append([]dbmodel.Log(nil), logs...)
		}
		cloned[i].Fields = fields
	}
	if cloned == nil {
		return logs, false
	}
	return cloned, true
}

func rewriteTagMap(tags map[string]any) (map[string]any, bool) {
	var cloned map[string]any
	for k, v := range tags {
		safe, changed := rewriteFloat(v)
		if !changed {
			continue
		}
		if cloned == nil {
			cloned = make(map[string]any, len(tags))
			for k2, v2 := range tags {
				cloned[k2] = v2
			}
		}
		cloned[k] = safe
	}
	if cloned == nil {
		return tags, false
	}
	return cloned, true
}

func (s *SpanWriter) splitElevatedTags(keyValues []dbmodel.KeyValue) ([]dbmodel.KeyValue, map[string]any) {
	if !s.allTagsAsFields && len(s.tagKeysAsFields) == 0 {
		return keyValues, nil
	}
	var tagsMap map[string]any
	var kvs []dbmodel.KeyValue
	for _, kv := range keyValues {
		// A non-finite float has to stay in the typed array. The flat tag object
		// has no type field, so elevating it would store a bare string and the
		// reader could not tell it from a string attribute of the same text.
		if _, nonFinite := rewriteFloat(kv.Value); nonFinite {
			kvs = append(kvs, kv)
			continue
		}
		if kv.Type != dbmodel.BinaryType && (s.allTagsAsFields || s.tagKeysAsFields[kv.Key]) {
			if tagsMap == nil {
				tagsMap = map[string]any{}
			}
			tagsMap[strings.ReplaceAll(kv.Key, ".", s.tagDotReplacement)] = kv.Value
		} else {
			kvs = append(kvs, kv)
		}
	}
	if kvs == nil {
		kvs = make([]dbmodel.KeyValue, 0)
	}
	return kvs, tagsMap
}
