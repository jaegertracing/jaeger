// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	traceIDAggregation   = "traceIDs"
	indexPrefixSeparator = "-"

	traceIDField           = "traceID"
	spanIDField            = "spanID"
	durationField          = "duration"
	startTimeField         = "startTime"
	startTimeMillisField   = "startTimeMillis"
	serviceNameField       = "process.serviceName"
	operationNameField     = "operationName"
	parentSpanIDField      = "parentSpanID"
	objectTagsField        = "tag"
	objectProcessTagsField = "process.tag"
	nestedTagsField        = "tags"
	nestedProcessTagsField = "process.tags"
	nestedLogFieldsField   = "logs.fields"
	tagKeyField            = "key"
	tagValueField          = "value"
	// numberSubField is the sub-field the typed-attribute mapping indexes a numeric
	// attribute value in, beside the keyword the same value is indexed as (RFC 0015).
	numberSubField = "number"
	errorTag       = "error"

	defaultSearchDepth = 100

	DawnOfTimeSpanAge = time.Hour * 24 * 365 * 50
)

var (
	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("start Time Minimum is above Maximum")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("duration Minimum is above Maximum")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("malformed request object")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("start and End Time must be set")

	// ErrUnableToFindTraceIDAggregation occurs when an aggregation query for TraceIDs fail.
	ErrUnableToFindTraceIDAggregation = errors.New("could not find aggregation of traceIDs")

	defaultMaxDuration = model.DurationAsMicroseconds(time.Hour * 24)

	objectTagFieldList = []string{objectTagsField, objectProcessTagsField}

	nestedTagFieldList = []string{nestedTagsField, nestedProcessTagsField, nestedLogFieldsField}

	_ Reader = (*SpanReader)(nil) // check API conformance
)

// Time-range design (referenced as "timeRangeDesign" in comments below):
//
// There are two read operations with different time-range semantics:
//
//  1. FindTraceIDs: the user always supplies [StartTimeMin, StartTimeMax]. No adjustment needed.
//
//  2. GetTraces (by trace ID): no time range is known. The reader uses [now-maxSpanAge, now].
//     - Periodic indices: maxSpanAge should match data retention (e.g., 7d). ReadTargets generates
//       only existing indices within that window.
//     - Alias indices: a single alias covers all data. The factory overrides maxSpanAge to
//       DawnOfTimeSpanAge (50 years) so the time-range filter doesn't exclude old traces.
//
// multiRead always adds a time-range filter to the ES query for shard pruning (helps ES skip
// irrelevant shards). This is harmless for periodic indices and essential for aliases.
//
// multiRead is called from both paths. When called from FindTraces, the time range is the user's
// search window — but a trace may extend beyond it. The ±maxTraceDuration padding on index
// selection and the ES time filter ensure all spans of a trace are found even when they extend
// beyond the search window.
//
// TODO: future improvement:
//   - With data streams, store a materialized view of (traceID, minStartTime, maxEndTime) to
//     enable precise time-range scoping for GetTraces (similar to ClickHouse approach).

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	searcher esclient.Searcher
	// maxSpanAge is how far back (in terms of timestamped indices)
	// we look when loading trace by ID (a query without a time range).
	maxSpanAge time.Duration
	// servicesMaxLookback bounds GetServices/GetOperations independently from maxSpanAge,
	// which may be widened to DawnOfTimeSpanAge for trace reads through an alias or data stream.
	servicesMaxLookback     time.Duration
	maxTraceDuration        time.Duration
	serviceOperationStorage *ServiceOperationStorage
	spanRotation            indices.Rotation
	serviceRotation         indices.Rotation
	maxDocCount             int
	logger                  *zap.Logger
	tracer                  trace.Tracer
	dotReplacer             dbmodel.DotReplacer
}

// SpanReaderParams holds constructor params for NewSpanReader
type SpanReaderParams struct {
	// Searcher is the esclient data-plane search client backing every read path:
	// service/operation reads, trace-ID and trace lookups, and native summaries.
	Searcher   esclient.Searcher
	MaxSpanAge time.Duration
	// ServicesMaxLookback bounds GetServices/GetOperations.
	ServicesMaxLookback time.Duration
	MaxTraceDuration    time.Duration
	MaxDocCount         int
	TagDotReplacement   string
	Logger              *zap.Logger
	Tracer              trace.Tracer
	SpanRotation        indices.Rotation
	ServiceRotation     indices.Rotation
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(p SpanReaderParams) *SpanReader {
	return &SpanReader{
		searcher:                p.Searcher,
		maxSpanAge:              p.MaxSpanAge,
		servicesMaxLookback:     p.ServicesMaxLookback,
		maxTraceDuration:        p.MaxTraceDuration,
		serviceOperationStorage: NewServiceOperationStorage(p.Searcher, p.Logger, 0), // read-only; the decorator takes care of metrics
		spanRotation:            p.SpanRotation,
		serviceRotation:         p.ServiceRotation,
		maxDocCount:             p.MaxDocCount,
		logger:                  p.Logger,
		tracer:                  p.Tracer,
		dotReplacer:             dbmodel.NewDotReplacer(p.TagDotReplacement),
	}
}

// traceReadCursor is the search_after pagination cursor for the per-trace read:
// the (startTime, spanID) pair of the last span of the previous page. spanID is
// the tie-breaker startTime alone cannot provide — spans routinely share a
// startTime (it has microsecond granularity, and SDKs that batch at millisecond
// precision emit many spans at the same instant), and search_after resumes
// strictly after the cursor, so paging on startTime alone silently drops every
// span that shares the boundary timestamp. Elasticsearch/OpenSearch require a
// unique tie-breaker field for a correct search_after; spanID is unique within a
// trace and stored as a sortable keyword.
type traceReadCursor struct {
	startTime uint64
	spanID    string
}

// buildTraceReadRequest builds the per-trace search body multiRead pages through:
// the trace's query, ordered by (startTime, spanID) ascending, with track_total_hits
// so the loop knows when a trace is fully fetched. The first page passes a nil
// cursor and omits search_after — the startTime range filter already bounds the
// lower end; follow-up pages pass the previous page's last (startTime, spanID) to
// resume. See traceReadCursor for why the spanID tie-breaker is required.
func (s *SpanReader) buildTraceReadRequest(q esquery.Query, cursor *traceReadCursor) esclient.SearchRequest {
	req := esclient.SearchRequest{
		Query: q,
		Size:  s.maxDocCount,
		Sort: []esclient.SortOrder{
			{Field: startTimeField, Order: esquery.Ascending},
			{Field: spanIDField, Order: esquery.Ascending},
		},
		TrackTotalHits: true,
	}
	if cursor != nil {
		req.SearchAfter = []any{cursor.startTime, cursor.spanID}
	}
	return req
}

// GetTraces takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTraces(ctx context.Context, query []dbmodel.TraceID) ([]dbmodel.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "GetTrace")
	defer span.End()
	currentTime := time.Now()
	// TODO: use start time & end time in "query" struct
	return s.multiRead(ctx, query, currentTime.Add(-s.maxSpanAge), currentTime)
}

func (s *SpanReader) collectSpans(esSpansRaw []esclient.SearchHit) ([]dbmodel.Span, error) {
	spans := make([]dbmodel.Span, len(esSpansRaw))

	for i, esSpanRaw := range esSpansRaw {
		dbSpan, err := s.unmarshalJSONSpan(esSpanRaw)
		if err != nil {
			return nil, fmt.Errorf("marshalling JSON to span object failed: %w", err)
		}
		s.mergeAllNestedAndElevatedTagsOfSpan(&dbSpan)
		spans[i] = dbSpan
	}
	return spans, nil
}

func (*SpanReader) unmarshalJSONSpan(esSpanRaw esclient.SearchHit) (dbmodel.Span, error) {
	esSpanInByteArray := esSpanRaw.Source

	var jsonSpan dbmodel.Span

	d := json.NewDecoder(bytes.NewReader(esSpanInByteArray))
	d.UseNumber()
	if err := d.Decode(&jsonSpan); err != nil {
		return dbmodel.Span{}, err
	}
	return jsonSpan, nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "GetService")
	defer span.End()
	currentTime := time.Now()
	jaegerIndices := s.serviceRotation.ReadTargets(currentTime.Add(-s.servicesMaxLookback), currentTime)
	return s.serviceOperationStorage.getServices(ctx, jaegerIndices, s.maxDocCount)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(
	ctx context.Context,
	query dbmodel.OperationQueryParameters,
) ([]dbmodel.Operation, error) {
	ctx, span := s.tracer.Start(ctx, "GetOperations")
	defer span.End()
	currentTime := time.Now()
	jaegerIndices := s.serviceRotation.ReadTargets(currentTime.Add(-s.servicesMaxLookback), currentTime)
	operations, err := s.serviceOperationStorage.getOperations(ctx, jaegerIndices, query.ServiceName, s.maxDocCount)
	if err != nil {
		return nil, err
	}

	// TODO: https://github.com/jaegertracing/jaeger/issues/1923
	// 	- return the operations with actual span kind that meet requirement
	var result []dbmodel.Operation
	for _, operation := range operations {
		result = append(result, dbmodel.Operation{
			Name: operation,
		})
	}
	return result, err
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) ([]dbmodel.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "FindTraces")
	defer span.End()

	page, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	return s.multiRead(ctx, page.TraceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReader) FindTraceIDs(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) (TraceIDPage, error) {
	ctx, span := s.tracer.Start(ctx, "FindTraceIDs")
	defer span.End()

	if err := validateQuery(traceQuery); err != nil {
		return TraceIDPage{}, err
	}
	if traceQuery.SearchDepth == 0 {
		traceQuery.SearchDepth = defaultSearchDepth
	}

	return s.findTraceIDsFromQuery(ctx, traceQuery)
}

func (s *SpanReader) multiRead(ctx context.Context, traceIDs []dbmodel.TraceID, startTime, endTime time.Time) ([]dbmodel.Trace, error) {
	ctx, childSpan := s.tracer.Start(ctx, "multiRead")
	defer childSpan.End()

	if childSpan.IsRecording() {
		tracesIDs := make([]string, len(traceIDs))
		for i, traceID := range traceIDs {
			tracesIDs[i] = string(traceID)
		}
		childSpan.SetAttributes(attribute.Key("trace_ids").StringSlice(tracesIDs))
	}

	traces := make([]dbmodel.Trace, 0, len(traceIDs))

	if len(traceIDs) == 0 {
		return traces, nil
	}

	// See timeRangeDesign above for context on the padding and the alias filter.
	idxList := s.spanRotation.ReadTargets(startTime.Add(-s.maxTraceDuration), endTime.Add(s.maxTraceDuration))
	searchAfter := make(map[dbmodel.TraceID]traceReadCursor)
	totalDocumentsFetched := make(map[dbmodel.TraceID]int)
	tracesMap := make(map[dbmodel.TraceID]*dbmodel.Trace)
	for len(traceIDs) != 0 {
		searchRequests := make([]esclient.MultiSearchRequest, len(traceIDs))
		for i, traceID := range traceIDs {
			traceQuery := buildTraceByIDQuery(traceID)
			startTimeRangeQuery := s.buildStartTimeQuery(startTime.Add(-s.maxTraceDuration), endTime.Add(s.maxTraceDuration))
			query := esquery.NewBoolQuery().
				Must(traceQuery).
				Must(startTimeRangeQuery)

			// First page sends no search_after; follow-up pages resume from the
			// previous page's last span.
			var cursor *traceReadCursor
			if val, ok := searchAfter[traceID]; ok {
				cursor = &val
			}

			searchRequests[i] = esclient.MultiSearchRequest{
				Indices: idxList,
				Search:  s.buildTraceReadRequest(query, cursor),
			}
		}
		// set traceIDs to empty
		traceIDs = nil
		responses, err := s.searcher.MultiSearch(ctx, searchRequests)
		if err != nil {
			logErrorToSpan(childSpan, err)
			return nil, err
		}

		if len(responses) == 0 {
			break
		}

		for _, result := range responses {
			// A failed _msearch item carries an error payload and no hits; skipping it
			// like an empty result would silently drop the trace (or truncate it, when
			// a later search_after page fails and the trace is never re-queued).
			if itemErr := result.Err(); itemErr != nil {
				err := fmt.Errorf("multi-search item failed: %w", itemErr)
				logErrorToSpan(childSpan, err)
				return nil, err
			}
			// Hits is a value (esclient.HitsResult), not a pointer, so there's no nil
			// to guard — only the inner slice can be empty.
			if len(result.Hits.Hits) == 0 {
				continue
			}
			spans, err := s.collectSpans(result.Hits.Hits)
			if err != nil {
				logErrorToSpan(childSpan, err)
				return nil, err
			}
			lastSpan := spans[len(spans)-1]

			if traceSpan, ok := tracesMap[lastSpan.TraceID]; ok {
				traceSpan.Spans = append(traceSpan.Spans, spans...)
			} else {
				traces = append(traces, dbmodel.Trace{Spans: spans})
				tracesMap[lastSpan.TraceID] = &traces[len(traces)-1]
			}

			totalDocumentsFetched[lastSpan.TraceID] += len(result.Hits.Hits)
			if totalDocumentsFetched[lastSpan.TraceID] < result.Hits.Total.Value {
				traceIDs = append(traceIDs, lastSpan.TraceID)
				searchAfter[lastSpan.TraceID] = traceReadCursor{
					startTime: lastSpan.StartTime,
					spanID:    string(lastSpan.SpanID),
				}
			}
		}
	}
	return traces, nil
}

func buildTraceByIDQuery(traceID dbmodel.TraceID) esquery.Query {
	return esquery.NewTermQuery(traceIDField, string(traceID))
}

func validateQuery(p dbmodel.TraceQueryParameters) error {
	if p.StartTimeMin.IsZero() || p.StartTimeMax.IsZero() {
		return ErrStartAndEndTimeNotSet
	}
	if p.StartTimeMax.Before(p.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return ErrDurationMinGreaterThanMax
	}
	return nil
}

// buildCursorFilter converts a traceCursor into a query-level boolean filter
// that encodes keyset semantics for the sort order (startTime DESC, traceID ASC).
//
// ES/OS prohibits multi-field sort when collapse and search_after are combined:
// the engine requires the sort to contain only the collapse field, so search_after
// cannot serve as the cursor here. Instead we express the same semantics as a
// query filter:
//
//	(startTime < T)
//	OR (startTime == T AND traceID > X)
//
// which is exactly the set of documents that come after position (T, X) in
// (startTime DESC, traceID ASC) order. The filter is applied to individual span
// documents before collapse, which is correct: a trace whose latest-starting span
// (its collapse representative) satisfies the condition appears on the next page;
// a trace whose latest span predates or ties at T with traceID ≤ X is excluded.
// Traces that have a span at time T (so their representative could be T) but whose
// traceID ≤ X are filtered on that span yet may still contribute a span below T
// that passes the first clause. Those potential duplicates are caught by the
// SeenIDs exclusion list encoded in the cursor (see findTraceIDsFromQuery).
func buildCursorFilter(c traceCursor) esquery.Query {
	// Clause 1: any span strictly before the cursor time.
	beforeCursor := esquery.NewRangeQuery(startTimeField).Lt(c.StartTime)
	// Clause 2: span at exactly the cursor time but with a later traceID.
	atCursorTime := esquery.NewTermQuery(startTimeField, c.StartTime)
	afterCursorID := esquery.NewRangeQuery(traceIDField).Gt(c.TraceID)
	atBoundary := esquery.NewBoolQuery().Must(atCursorTime, afterCursorID)
	return esquery.NewBoolQuery().Should(beforeCursor, atBoundary)
}

func (s *SpanReader) findTraceIDsFromQuery(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) (TraceIDPage, error) {
	ctx, childSpan := s.tracer.Start(ctx, "findTraceIDs")
	defer childSpan.End()

	boolQuery, err := s.buildFindTraceIDsBoolQuery(traceQuery)
	if err != nil {
		return TraceIDPage{}, err
	}
	jaegerIndices := s.spanRotation.ReadTargets(traceQuery.StartTimeMin, traceQuery.StartTimeMax)

	// seenIDs is the set of traceIDs from the previous page that share the cursor's
	// startTime. The cursor filter's span-level "startTime < T" clause may surface
	// a span of such a trace at a time below T, causing collapse to include it with
	// a lower representative — duplicating it across pages. We remove those here.
	var seenIDs map[string]struct{}

	if traceQuery.PageToken != "" {
		cursor, err := decodeTraceCursor(traceQuery.PageToken)
		if err != nil {
			return TraceIDPage{}, err
		}
		// Inject the keyset cursor as a query filter instead of search_after.
		// See buildCursorFilter for the rationale.
		boolQuery.Must(buildCursorFilter(cursor))
		if len(cursor.SeenIDs) > 0 {
			seenIDs = make(map[string]struct{}, len(cursor.SeenIDs))
			for _, id := range cursor.SeenIDs {
				seenIDs[id] = struct{}{}
			}
		}
	}

	searchReq := esclient.SearchRequest{
		Size:  traceQuery.SearchDepth,
		Query: boolQuery,
		Collapse: &esclient.Collapse{
			Field: traceIDField,
		},
		Sort: []esclient.SortOrder{
			{Field: startTimeField, Order: esquery.Descending},
			{Field: traceIDField, Order: esquery.Ascending},
		},
	}

	searchResult, err := s.searcher.Search(ctx, jaegerIndices, searchReq)
	if err != nil {
		s.logger.Info("es search services failed", zap.Any("traceQuery", traceQuery), zap.Error(err))
		return TraceIDPage{}, fmt.Errorf("search services failed: %w", err)
	}

	hits := searchResult.Hits.Hits
	traceIDs := make([]dbmodel.TraceID, 0, len(hits))
	for _, hit := range hits {
		var src struct {
			TraceID string `json:"traceID"`
		}
		if err := json.Unmarshal(hit.Source, &src); err != nil || src.TraceID == "" {
			continue
		}
		// Drop any trace that appeared on the previous page at the cursor's
		// startTime boundary; see buildCursorFilter for why this is necessary.
		if _, dup := seenIDs[src.TraceID]; dup {
			continue
		}
		traceIDs = append(traceIDs, dbmodel.TraceID(src.TraceID))
	}

	var nextPageToken string
	if len(hits) > 0 && len(hits[len(hits)-1].Sort) >= 2 {
		lastHit := hits[len(hits)-1]
		var lastStartTime uint64
		switch v := lastHit.Sort[0].(type) {
		case float64:
			lastStartTime = uint64(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				lastStartTime = uint64(n)
			}
		}
		var lastTraceID string
		if tid, ok := lastHit.Sort[1].(string); ok {
			lastTraceID = tid
		}
		if lastTraceID != "" {
			// Collect all traceIDs on this page that share the last hit's startTime.
			// These are candidates for appearing on the next page via a lower-time
			// span after the cursor filter is applied, and must be excluded there.
			var boundary []string
			for _, tid := range traceIDs {
				if sortTimeForTraceID(hits, string(tid)) == lastStartTime {
					boundary = append(boundary, string(tid))
				}
			}
			nextPageToken = encodeTraceCursor(traceCursor{
				StartTime: lastStartTime,
				TraceID:   lastTraceID,
				SeenIDs:   boundary,
			})
		}
	}

	return TraceIDPage{
		TraceIDs:      traceIDs,
		NextPageToken: nextPageToken,
	}, nil
}

// sortTimeForTraceID returns the sort startTime value of the hit whose traceID
// matches tid, or 0 if not found.
func sortTimeForTraceID(hits []esclient.SearchHit, tid string) uint64 {
	for _, h := range hits {
		if len(h.Sort) < 2 {
			continue
		}
		hitTID, ok := h.Sort[1].(string)
		if !ok || hitTID != tid {
			continue
		}
		switch v := h.Sort[0].(type) {
		case float64:
			return uint64(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				return uint64(n)
			}
		}
	}
	return 0
}

// traceCursor is the keyset cursor for FindTraceIDs pagination.
// StartTime and TraceID define the (startTime DESC, traceID ASC) position of
// the last result on the previous page. SeenIDs is the set of traceIDs from
// that page that share StartTime exactly; they are excluded from the next page
// to prevent duplicates that can arise when the cursor filter is applied at the
// individual-span level rather than the trace-representative level.
type traceCursor struct {
	StartTime uint64   `json:"st"`
	TraceID   string   `json:"tid"`
	SeenIDs   []string `json:"seen,omitempty"`
}

func encodeTraceCursor(c traceCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeTraceCursor(token string) (traceCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return traceCursor{}, fmt.Errorf("invalid page token: %w", err)
	}
	var c traceCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return traceCursor{}, fmt.Errorf("invalid page token: %w", err)
	}
	if c.TraceID == "" {
		return traceCursor{}, fmt.Errorf("invalid page token: missing trace ID")
	}
	return c, nil
}

func (s *SpanReader) buildFindTraceIDsBoolQuery(traceQuery dbmodel.TraceQueryParameters) (*esquery.BoolQuery, error) {
	boolQuery := esquery.NewBoolQuery()

	// add duration query
	if traceQuery.DurationMax != 0 || traceQuery.DurationMin != 0 {
		durationQuery := s.buildDurationQuery(traceQuery.DurationMin, traceQuery.DurationMax)
		boolQuery.Must(durationQuery)
	}

	// add startTime query
	startTimeQuery := s.buildStartTimeQuery(traceQuery.StartTimeMin, traceQuery.StartTimeMax)
	boolQuery.Must(startTimeQuery)

	// add process.serviceName query
	if traceQuery.ServiceName != "" {
		serviceNameQuery := s.buildServiceNameQuery(traceQuery.ServiceName)
		boolQuery.Must(serviceNameQuery)
	}

	// add operationName query
	if traceQuery.OperationName != "" {
		operationNameQuery := s.buildOperationNameQuery(traceQuery.OperationName)
		boolQuery.Must(operationNameQuery)
	}

	for k, v := range traceQuery.Tags {
		// The error tag is only written for error spans (error=true; see
		// getTagFromStatusCode in to_dbmodel.go). A non-error span carries no error
		// tag at all, so a literal error=false tag match returns nothing. Treat
		// error=false as the complement of error=true — every non-error span — by
		// excluding error=true instead, mirroring the in-memory store (#9096).
		if k == errorTag {
			if isError, parseErr := strconv.ParseBool(v); parseErr == nil {
				if isError {
					boolQuery.Must(s.buildTagQuery(errorTag, "true"))
				} else {
					boolQuery.MustNot(s.buildTagQuery(errorTag, "true"))
				}
				continue
			}
		}
		tagQuery := s.buildTagQuery(k, v)
		boolQuery.Must(tagQuery)
	}

	// The structured filter carries the same kinds of predicate as the fields above and the
	// query service keeps the two mutually exclusive, so at most one of them contributes
	// clauses to this query.
	if traceQuery.Filter != nil {
		filterQuery, err := s.buildFilterQuery(traceQuery.Filter)
		if err != nil {
			return nil, err
		}
		boolQuery.Must(filterQuery)
	}
	return boolQuery, nil
}

func (*SpanReader) buildDurationQuery(durationMin time.Duration, durationMax time.Duration) esquery.Query {
	minDurationMicros := model.DurationAsMicroseconds(durationMin)
	maxDurationMicros := defaultMaxDuration
	if durationMax != 0 {
		maxDurationMicros = model.DurationAsMicroseconds(durationMax)
	}
	return esquery.NewRangeQuery(durationField).Gte(minDurationMicros).Lte(maxDurationMicros)
}

func (*SpanReader) buildStartTimeQuery(startTimeMin time.Time, startTimeMax time.Time) esquery.Query {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin)
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMax)
	// startTimeMillisField is date field in ES mapping.
	// Using date field in range queries helps to skip search on unnecessary shards at Elasticsearch side.
	// https://discuss.elastic.co/t/timeline-query-on-timestamped-indices/129328/2
	return esquery.NewRangeQuery(startTimeMillisField).Gte(minStartTimeMicros / 1000).Lte(maxStartTimeMicros / 1000)
}

func (*SpanReader) buildServiceNameQuery(serviceName string) esquery.Query {
	return esquery.NewMatchQuery(serviceNameField, serviceName)
}

func (*SpanReader) buildOperationNameQuery(operationName string) esquery.Query {
	return esquery.NewMatchQuery(operationNameField, operationName)
}

func (s *SpanReader) buildTagQuery(k string, v string) esquery.Query {
	objectTagListLen := len(objectTagFieldList)
	queries := make([]esquery.Query, len(nestedTagFieldList)+objectTagListLen)
	kd := s.dotReplacer.ReplaceDot(k)
	for i := range objectTagFieldList {
		queries[i] = s.buildObjectQuery(objectTagFieldList[i], kd, v)
	}
	for i := range nestedTagFieldList {
		queries[i+objectTagListLen] = s.buildNestedQuery(nestedTagFieldList[i], k, v)
	}

	// but configuration can change over time
	return esquery.NewBoolQuery().Should(queries...)
}

func (*SpanReader) buildNestedQuery(field string, k string, v string) esquery.Query {
	keyField := fmt.Sprintf("%s.%s", field, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", field, tagValueField)
	keyQuery := esquery.NewMatchQuery(keyField, k)
	valueQuery := esquery.NewRegexpQuery(valueField, v).Flags("NONE")
	tagBoolQuery := esquery.NewBoolQuery().Must(keyQuery, valueQuery)
	return esquery.NewNestedQuery(field, tagBoolQuery)
}

func (*SpanReader) buildObjectQuery(field string, k string, v string) esquery.Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := esquery.NewRegexpQuery(keyField, v).Flags("NONE")
	return esquery.NewBoolQuery().Must(keyQuery)
}

func (s *SpanReader) mergeAllNestedAndElevatedTagsOfSpan(span *dbmodel.Span) {
	processTags := s.mergeNestedAndElevatedTags(span.Process.Tags, span.Process.Tag)
	span.Process.Tags = processTags
	spanTags := s.mergeNestedAndElevatedTags(span.Tags, span.Tag)
	span.Tags = spanTags
}

func (s *SpanReader) mergeNestedAndElevatedTags(nestedTags []dbmodel.KeyValue, elevatedTags map[string]any) []dbmodel.KeyValue {
	mergedTags := make([]dbmodel.KeyValue, 0, len(nestedTags)+len(elevatedTags))
	mergedTags = append(mergedTags, nestedTags...)
	for k, v := range elevatedTags {
		kv := s.convertTagField(k, v)
		mergedTags = append(mergedTags, kv)
		delete(elevatedTags, k)
	}
	return mergedTags
}

func (s *SpanReader) convertTagField(k string, v any) dbmodel.KeyValue {
	dKey := s.dotReplacer.ReplaceDotReplacement(k)
	kv := dbmodel.KeyValue{
		Key:   dKey,
		Value: v,
	}
	switch val := v.(type) {
	case int64:
		kv.Type = dbmodel.Int64Type
	case float64:
		kv.Type = dbmodel.Float64Type
	case bool:
		kv.Type = dbmodel.BoolType
	case string:
		kv.Type = dbmodel.StringType
	// the binary is never returned, ES returns it as string with base64 encoding
	case []byte:
		kv.Type = dbmodel.BinaryType
	// in spans are decoded using json.UseNumber() to preserve the type
	// however note that float(1) will be parsed as int as ES does not store decimal point
	case json.Number:
		n, err := val.Int64()
		if err == nil {
			kv.Value = n
			kv.Type = dbmodel.Int64Type
		} else {
			f, err := val.Float64()
			if err != nil {
				return dbmodel.KeyValue{
					Key:   dKey,
					Value: fmt.Sprintf("invalid tag type in %+v: %s", v, err.Error()),
					Type:  dbmodel.StringType,
				}
			}
			kv.Value = f
			kv.Type = dbmodel.Float64Type
		}
	default:
		return dbmodel.KeyValue{
			Key:   dKey,
			Value: fmt.Sprintf("invalid tag type in %+v", v),
			Type:  dbmodel.StringType,
		}
	}
	return kv
}

func logErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
