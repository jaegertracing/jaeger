// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

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

// spanCursor is where a page of a span search ended: the sort values of the page's last hit,
// as the engine returned them, and the documents already returned that share those values. A
// continuation asks for the documents whose sort key is at or after the values and excludes
// those documents, so a stored duplicate that ties with the last hit on every sort field is
// still returned on the next page rather than skipped, which a bare search_after would do
// (RFC 0016 §6.4). Naming the documents is what makes a count of returned copies unnecessary:
// the engine does not promise a repeatable order among tied hits, but the names are exact.
type spanCursor struct {
	Sort []json.RawMessage `json:"sort"`
	Docs []spanDoc         `json:"docs"`
}

// spanDoc names one stored document. The id alone does not: a span retried after a rollover
// can be stored under the same id in two backing indices, and both are occurrences.
type spanDoc struct {
	Index string `json:"index"`
	ID    string `json:"id"`
}

// FindSpans returns one page of the spans matching the query, in the query's effective
// order, and the cursor that resumes the search after them (RFC 0016 §6). The engine sorts
// and cuts the page, so a page reads one document past what it returns, and that extra hit
// only tells whether another page exists. The engine returns at most maxDocCount hits, the
// index's result window, so a page that would need more is cut one short of the window; a
// page size is a maximum, and the cursor still resumes exactly after the page.
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
	after, err := decodeSpanCursor(query.Cursor, order)
	if err != nil {
		return dbmodel.SpanPage{}, err
	}
	// A fetch of one hit could not tell a full page from the end of the results, so the
	// configured limit is raised to two where it is lower.
	fetch := min(query.PageSize+1, max(s.maxDocCount, 2))
	req, err := s.buildSpanSearchRequest(query, order, after, fetch)
	if err != nil {
		return dbmodel.SpanPage{}, err
	}
	indices := s.spanRotation.ReadTargets(query.StartTimeMin, query.StartTimeMax)
	result, err := s.searcher.Search(ctx, indices, req)
	if err != nil {
		s.logger.Info("es span search failed", zap.Error(err))
		logErrorToSpan(span, err)
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
		next, err := nextSpanCursor(hits, after, len(order))
		if err != nil {
			logErrorToSpan(span, err)
			return dbmodel.SpanPage{}, err
		}
		if page.NextCursor, err = json.Marshal(next); err != nil {
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

// buildSpanSearchRequest builds the search body of one page: the time range, the filter,
// and on a continuation the keyset predicate and id exclusion of the cursor, sorted by the
// effective order and sized to fetch hits.
func (s *SpanReader) buildSpanSearchRequest(
	query dbmodel.SpanQueryParameters,
	order []tracestore.SpanSortOrder,
	after *spanCursor,
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
	if after != nil {
		boolQuery.Must(keysetAtOrAfter(order, after.Sort))
		boolQuery.MustNot(returnedDocs(after.Docs)...)
	}
	sort := make([]esclient.SortOrder, len(order))
	for i, term := range order {
		sort[i] = esclient.SortOrder{Field: sortField(term), Order: sortDirection(term.Direction)}
	}
	return esclient.SearchRequest{
		Query: boolQuery,
		Size:  fetch,
		Sort:  sort,
	}, nil
}

// sortField is the document field an ordering term sorts on. The order arrives settled by
// tracestore.EffectiveSpanOrder, which admits only the fields spanSortFields maps.
func sortField(term tracestore.SpanSortOrder) string {
	ref, ok := term.Expression.(*expression.FieldRef)
	if !ok || spanSortFields[ref.Name] == "" {
		panic(fmt.Sprintf("ordering term %v escaped tracestore.EffectiveSpanOrder", term.Expression))
	}
	return spanSortFields[ref.Name]
}

// returnedDocs selects the documents a cursor names, one ids query per backing index, since
// an id identifies a document only together with its index.
func returnedDocs(docs []spanDoc) []esquery.Query {
	byIndex := make(map[string][]string)
	var indices []string
	for _, doc := range docs {
		if _, seen := byIndex[doc.Index]; !seen {
			indices = append(indices, doc.Index)
		}
		byIndex[doc.Index] = append(byIndex[doc.Index], doc.ID)
	}
	queries := make([]esquery.Query, len(indices))
	for i, index := range indices {
		queries[i] = esquery.NewBoolQuery().Must(esquery.NewTermQuery("_index", index), esquery.NewIdsQuery(byIndex[index]...))
	}
	return queries
}

func sortDirection(direction tracestore.SortDirection) esquery.SortDirection {
	if direction == tracestore.SortDescending {
		return esquery.Descending
	}
	return esquery.Ascending
}

// keysetAtOrAfter selects the documents whose sort key is equal to or comes after key in the
// given order: for some prefix of the terms the document equals the key, and on the next term
// it is past the key in that term's direction, or it equals the key on every term. The last
// alternative is what admits the documents that tie with the page's last hit, which the
// cursor's ids then thin out.
func keysetAtOrAfter(order []tracestore.SpanSortOrder, key []json.RawMessage) esquery.Query {
	alternatives := make([]esquery.Query, 0, len(order)+1)
	for i, term := range order {
		past := esquery.NewRangeQuery(sortField(term))
		if term.Direction == tracestore.SortDescending {
			past.Lt(key[i])
		} else {
			past.Gt(key[i])
		}
		alternatives = append(alternatives, esquery.NewBoolQuery().Must(append(keysetEquals(order[:i], key), past)...))
	}
	alternatives = append(alternatives, esquery.NewBoolQuery().Must(keysetEquals(order, key)...))
	return esquery.NewBoolQuery().Should(alternatives...)
}

func keysetEquals(order []tracestore.SpanSortOrder, key []json.RawMessage) []esquery.Query {
	equals := make([]esquery.Query, len(order))
	for i, term := range order {
		equals[i] = esquery.NewTermQuery(sortField(term), key[i])
	}
	return equals
}

// decodeSpanCursor reads the cursor a token carried, or returns nil for none. A cursor is
// refused unless it holds one sort value per term of the order, each a number for a long field
// and a string for a keyword field, and names at least one document, since the reader never
// returns one shaped otherwise and the engine would reject the query built from it.
func decodeSpanCursor(raw []byte, order []tracestore.SpanSortOrder) (*spanCursor, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var c spanCursor
	if err := json.Unmarshal(raw, &c); err != nil || len(c.Sort) != len(order) || len(c.Docs) == 0 {
		return nil, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	for i, term := range order {
		var want any
		switch sortField(term) {
		case traceIDField, spanIDField:
			want = new(string)
		default:
			want = new(json.Number)
		}
		// A JSON null decodes into either target without error, so it is refused on its own.
		if err := json.Unmarshal(c.Sort[i], want); err != nil || bytes.Equal(bytes.TrimSpace(c.Sort[i]), []byte("null")) {
			return nil, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
		}
	}
	return &c, nil
}

// nextSpanCursor is the cursor that resumes after the page's last hit: its sort values, with
// every hit on the page that shares them, and the documents the previous cursor named when the
// page did not move past its key.
func nextSpanCursor(page []esclient.SearchHit, after *spanCursor, terms int) (*spanCursor, error) {
	last := page[len(page)-1]
	if len(last.Sort) != terms {
		return nil, fmt.Errorf("span search returned %d sort values for %d sort fields", len(last.Sort), terms)
	}
	next := &spanCursor{Sort: last.Sort}
	for _, hit := range page {
		if sameSortValues(hit.Sort, last.Sort) {
			next.Docs = append(next.Docs, spanDoc{Index: hit.Index, ID: hit.ID})
		}
	}
	if after != nil && sameSortValues(after.Sort, last.Sort) {
		next.Docs = append(next.Docs, after.Docs...)
	}
	return next, nil
}

// sameSortValues compares two sort keys by their JSON values, so a key read back from a token
// compares equal to the engine's encoding of the same values however either was formatted.
func sameSortValues(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	var ca, cb bytes.Buffer
	for i := range a {
		ca.Reset()
		cb.Reset()
		if json.Compact(&ca, a[i]) != nil || json.Compact(&cb, b[i]) != nil || !bytes.Equal(ca.Bytes(), cb.Bytes()) {
			return false
		}
	}
	return true
}
