// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// A span search appends the engine's metadata fields to its sort so that the sort key names
// one document (RFC 0016 §6.4): indexField always, since a document is unique only within its
// index, and idField when SpanReaderParams.SpanSearchTieBreakByID is set.
const (
	indexField = "_index"
	idField    = "_id"
)

// idFieldDataSetting is the cluster setting that allows sorting on _id. Elasticsearch names it
// in the error it returns when the setting is off, which is how the reader recognizes that
// failure.
const idFieldDataSetting = "indices.id_field_data.enabled"

// spanSortFields maps an ordering term's built-in span field to the document field that
// carries it. Every field is a single-valued long or keyword, so its sort order is the
// contract's order (RFC 0016 §6.3): startTime and duration are microsecond longs, and the
// IDs are fixed-width lower-case hex keywords whose bytewise order is their numeric order.
var spanSortFields = map[string]string{
	expression.SpanFieldStartTime: startTimeField,
	expression.SpanFieldDuration:  durationField,
	expression.SpanFieldTraceID:   traceIDField,
	expression.SpanFieldSpanID:    spanIDField,
}

// FindSpans returns one page of the spans matching the query, in the query's effective
// order, and the cursor that resumes the search after them (RFC 0016 §6). The cursor is the
// last hit's sort values as the engine returned them, and a continuation passes them back as
// search_after. The engine sorts and cuts the page, so a page reads one document past what
// it returns, and that extra hit only tells whether another page exists. The engine returns
// at most maxDocCount hits, the index's result window, so a page that would need more is cut
// one short of the window; a page size is a maximum, and the cursor still resumes after the
// page. The sort ends in the backing index and, when the reader is configured to sort on it,
// the document id, so that documents tying on every public field are still returned once
// each across a page boundary (RFC 0016 §6.4); without the id, the occurrences after the
// boundary within one index are lost.
func (s *SpanReader) FindSpans(ctx context.Context, query dbmodel.SpanQueryParameters) (dbmodel.SpanPage, error) {
	ctx, span := s.tracer.Start(ctx, "FindSpans")
	defer span.End()

	order, err := tracestore.EffectiveSpanOrder(query.OrderBy)
	if err != nil {
		return dbmodel.SpanPage{}, err
	}
	if err := validateSpanQuery(query); err != nil {
		return dbmodel.SpanPage{}, err
	}
	sort, err := s.spanSort(order)
	if err != nil {
		return dbmodel.SpanPage{}, err
	}
	after, err := decodeSpanCursor(query.Cursor, sort)
	if err != nil {
		return dbmodel.SpanPage{}, err
	}
	// A fetch of one hit could not tell a full page from the end of the results, so the
	// configured limit is raised to two where it is lower.
	fetch := min(query.PageSize+1, max(s.maxDocCount, 2))
	req, err := s.buildSpanSearchRequest(query, sort, after, fetch)
	if err != nil {
		return dbmodel.SpanPage{}, err
	}
	indices := s.spanRotation.ReadTargets(query.StartTimeMin, query.StartTimeMax)
	result, err := s.searcher.Search(ctx, indices, req)
	if err != nil {
		s.logger.Info("es span search failed", zap.Error(err))
		logErrorToSpan(span, err)
		if s.spanSearchTieBreakByID && strings.Contains(err.Error(), idFieldDataSetting) {
			return dbmodel.SpanPage{}, fmt.Errorf(
				"span search sorts on the document _id (span_search_tie_break_by_id is on), which this cluster forbids: "+
					"enable the cluster setting %s or set span_search_tie_break_by_id to false: %w", idFieldDataSetting, err)
		}
		return dbmodel.SpanPage{}, fmt.Errorf("span search failed: %w", err)
	}
	hits := result.Hits.Hits
	more := len(hits) >= fetch
	if more {
		hits = hits[:fetch-1]
	}
	spans, err := s.collectSpans(hits)
	if err != nil {
		logErrorToSpan(span, err)
		return dbmodel.SpanPage{}, err
	}
	page := dbmodel.SpanPage{Spans: spans}
	if more {
		last := hits[len(hits)-1]
		if len(last.Sort) != len(sort) {
			err := fmt.Errorf("span search returned %d sort values for %d sort fields", len(last.Sort), len(sort))
			logErrorToSpan(span, err)
			return dbmodel.SpanPage{}, err
		}
		if page.NextCursor, err = json.Marshal(last.Sort); err != nil {
			return dbmodel.SpanPage{}, err
		}
	}
	return page, nil
}

func validateSpanQuery(query dbmodel.SpanQueryParameters) error {
	if query.StartTimeMin.IsZero() || query.StartTimeMax.IsZero() {
		return ErrStartAndEndTimeNotSet
	}
	if query.StartTimeMax.Before(query.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if query.PageSize <= 0 {
		return fmt.Errorf("%w: page size must be greater than 0", tracestore.ErrPaginationInvalid)
	}
	return nil
}

// spanSort lowers the effective order to the engine's sort clauses and appends the
// (_index, _id) tie-breaker of RFC 0016 §6.4, the id only when the reader is configured to
// sort on it, which it is not by default. Without the id, spans that tie on every ordering
// term within one index and straddle a page boundary are skipped by the next page, since
// search_after resumes strictly after the cursor's key. The index sorts descending so that,
// under the default start-time order, the newer of two indices holding the same key comes
// first.
func (s *SpanReader) spanSort(order []tracestore.SpanSortOrder) ([]esclient.SortOrder, error) {
	sort := make([]esclient.SortOrder, 0, len(order)+2)
	for _, term := range order {
		field, err := sortField(term)
		if err != nil {
			return nil, err
		}
		sort = append(sort, esclient.SortOrder{Field: field, Order: sortDirection(term.Direction)})
	}
	sort = append(sort, esclient.SortOrder{Field: indexField, Order: esquery.Descending})
	if s.spanSearchTieBreakByID {
		sort = append(sort, esclient.SortOrder{Field: idField, Order: esquery.Ascending})
	}
	return sort, nil
}

// sortField is the document field an ordering term sorts on. The order arrives settled by
// tracestore.EffectiveSpanOrder, which admits only the fields spanSortFields maps.
func sortField(term tracestore.SpanSortOrder) (string, error) {
	ref, ok := term.Expression.(*expression.FieldRef)
	if !ok {
		return "", errors.New("ordering term is not a field ref")
	}
	if spanSortFields[ref.Name] == "" {
		return "", fmt.Errorf("unsupported ordering term '%v'", ref.Name)
	}
	return spanSortFields[ref.Name], nil
}

func sortDirection(direction tracestore.SortDirection) esquery.SortDirection {
	if direction == tracestore.SortDescending {
		return esquery.Descending
	}
	return esquery.Ascending
}

// buildSpanSearchRequest builds the search body of one page: the time range and the filter,
// sorted by the given clauses, resumed after the cursor when there is one, and sized to fetch
// hits.
func (s *SpanReader) buildSpanSearchRequest(
	query dbmodel.SpanQueryParameters,
	sort []esclient.SortOrder,
	after []json.RawMessage,
	fetch int,
) (esclient.SearchRequest, error) {
	// The millisecond range prunes shards; the microsecond range is the bound itself, since the
	// span's stored start time tells apart spans the millisecond field does not.
	boolQuery := esquery.NewBoolQuery().Must(
		s.buildStartTimeQuery(query.StartTimeMin, query.StartTimeMax),
		esquery.NewRangeQuery(startTimeField).
			Gte(model.TimeAsEpochMicroseconds(query.StartTimeMin)).
			Lte(model.TimeAsEpochMicroseconds(query.StartTimeMax)),
	)
	if query.Filter != nil {
		filterQuery, err := s.buildFilterQuery(query.Filter)
		if err != nil {
			return esclient.SearchRequest{}, err
		}
		boolQuery.Must(filterQuery)
	}
	req := esclient.SearchRequest{
		Query: boolQuery,
		Size:  fetch,
		Sort:  sort,
	}
	for _, value := range after {
		req.SearchAfter = append(req.SearchAfter, value)
	}
	return req, nil
}

// decodeSpanCursor reads the cursor a token carried, or returns nil for none. A cursor is
// refused unless it holds one value per sort clause, a number for a long field and a string
// for a keyword or a metadata field, since the reader never returns one shaped otherwise and
// the engine would reject the search_after built from it. The two refusals carry distinct
// messages so that a test of one cannot pass by tripping the other.
func decodeSpanCursor(raw []byte, sort []esclient.SortOrder) ([]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	if len(values) != len(sort) {
		return nil, fmt.Errorf("%w: page token carries %d values for %d sort fields", tracestore.ErrPaginationInvalid, len(values), len(sort))
	}
	for i, clause := range sort {
		var want any
		kind := "number"
		switch clause.Field {
		case traceIDField, spanIDField, indexField, idField:
			want = new(string)
			kind = "string"
		default:
			want = new(json.Number)
		}
		// A JSON null decodes into either target without error, so it is refused on its own.
		if err := json.Unmarshal(values[i], want); err != nil || bytes.Equal(bytes.TrimSpace(values[i]), []byte("null")) {
			return nil, fmt.Errorf("%w: page token value %d is not a %s", tracestore.ErrPaginationInvalid, i, kind)
		}
	}
	return values, nil
}
