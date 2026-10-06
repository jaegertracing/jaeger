// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// SpanLookupParams names specific spans of one trace by span ID.
type SpanLookupParams struct {
	TraceID pcommon.TraceID
	// SpanIDs are the spans to find. Every ID must be non-zero: a zero ID never identifies a span.
	SpanIDs []pcommon.SpanID
}

// SpanMatch is one requested span together with the resource it was reported under.
type SpanMatch struct {
	Resource pcommon.Resource
	Span     ptrace.Span
}

// SpanLookupResult is the answer to a SpanLookupParams.
type SpanLookupResult struct {
	// Spans are the requested spans that were found, adjusted the way GetTraces adjusts them.
	Spans []SpanMatch
	// TraceFound reports whether the trace exists. The identity-filter path cannot tell a missing
	// trace from a filter that matched nothing without reading the whole trace, which is the cost
	// that path exists to avoid, so it reports true.
	TraceFound bool
}

// LookupSpans finds specific spans of one trace (RFC 0016 §4.3). A backend that declares span
// search answers with exactly the matching spans through FindSpans. Otherwise, or when archive
// storage is configured (a span search cannot see archived traces), it reads the whole trace
// through GetTraces and picks the spans out of it.
//
// The fast path errors out before any span is reported whenever it cannot run at all, so falling
// back to the whole-trace read afterward is safe. A refusal of the identity filter itself comes
// after OnSpanQuery, which may have narrowed the filter for a policy; falling back then would read
// the trace without that narrowing, so it is only retried when no interceptor is configured.
func (qs QueryService) LookupSpans(ctx context.Context, params SpanLookupParams) (SpanLookupResult, error) {
	if qs.hasArchiveTraceReader() {
		return qs.lookupViaGetTraces(ctx, params)
	}
	spans, err := qs.lookupViaFindSpans(ctx, params)
	if err != nil {
		if qs.canFallBackToGetTraces(err) {
			return qs.lookupViaGetTraces(ctx, params)
		}
		return SpanLookupResult{}, err
	}
	return SpanLookupResult{Spans: spans, TraceFound: true}, nil
}

// canFallBackToGetTraces reports whether a fast-path error may be answered by the whole-trace path.
func (qs QueryService) canFallBackToGetTraces(err error) bool {
	if errors.Is(err, ErrSpanSearchUnsupported) || errors.Is(err, ErrFilterDisabled) {
		return true
	}
	return errors.Is(err, tracestore.ErrFilterUnsupported) && !qs.hasInterceptors()
}

// adjustSpans applies the adjusters the whole-trace path applies. A span search returns stored
// spans unadjusted, so a caller that reports them the way GetTraces would must adjust them first.
func (qs QueryService) adjustSpans(traces ptrace.Traces) {
	qs.adjuster.Adjust(traces)
}

// hasArchiveTraceReader reports whether archive storage is configured. The whole-trace path
// reads it for traces the primary reader does not have; a span search does not, so a caller
// must not rely on a span search alone when this is true.
func (qs QueryService) hasArchiveTraceReader() bool {
	return qs.options.ArchiveTraceReader != nil
}

// hasInterceptors reports whether a query interceptor is configured. A caller uses it to decide
// whether a refusal that comes after the interceptors ran can be retried on another path.
func (qs QueryService) hasInterceptors() bool {
	return len(qs.options.Interceptors) > 0
}

// lookupViaFindSpans runs the identity-filter fast path. It uses the widest possible time range
// because a lookup by trace and span ID carries no time hint, and a span search requires one
// regardless of the filter (RFC 0016 §4.5, "a caller that knows only trace IDs supplies a range
// wide enough to contain them").
func (qs QueryService) lookupViaFindSpans(ctx context.Context, params SpanLookupParams) ([]SpanMatch, error) {
	spanIDStrs := make([]string, 0, len(params.SpanIDs))
	pending := make(map[pcommon.SpanID]struct{}, len(params.SpanIDs))
	for _, spanID := range params.SpanIDs {
		spanIDStrs = append(spanIDStrs, spanID.String())
		pending[spanID] = struct{}{}
	}
	filter := buildIdentityFilter(params.TraceID, spanIDStrs)

	var spans []SpanMatch
	// The default page size is 100 and the caller may name more spans than that, so every page is
	// read until the token runs out. A repeated token would loop forever, so it ends the read too.
	var pageSize uint32
	if n := len(params.SpanIDs); n <= math.MaxUint32 {
		pageSize = uint32(n)
	}
	var pageToken string
	for {
		query := SpanQueryParams{
			StartTimeMin: time.Unix(0, 0),
			StartTimeMax: time.Now(),
			Filter:       filter,
			Pagination:   Pagination{PageSize: pageSize, PageToken: pageToken},
		}
		var nextToken string
		for chunk, err := range qs.FindSpans(ctx, query) {
			if err != nil {
				return nil, err
			}
			// A span search returns stored spans unadjusted, so apply the same adjusters the
			// whole-trace path applies before anything reports them.
			qs.adjustSpans(chunk.Results)
			for pos, span := range jptrace.SpanIter(chunk.Results) {
				// Span IDs are unique only within a trace. An interceptor may have rewritten the
				// filter, so a match must also belong to the trace that was asked for.
				if span.TraceID() != params.TraceID {
					continue
				}
				if _, found := pending[span.SpanID()]; found {
					spans = append(spans, SpanMatch{Resource: pos.Resource.Resource(), Span: span})
					delete(pending, span.SpanID())
				}
			}
			nextToken = string(chunk.NextPageToken)
		}
		if nextToken == "" || nextToken == pageToken || len(pending) == 0 {
			return spans, nil
		}
		pageToken = nextToken
	}
}

// lookupViaGetTraces is the whole-trace path: it reads the trace, archive included, and picks the
// requested spans out of it in memory. Every backend supports it.
func (qs QueryService) lookupViaGetTraces(ctx context.Context, params SpanLookupParams) (SpanLookupResult, error) {
	pending := make(map[pcommon.SpanID]struct{}, len(params.SpanIDs))
	for _, spanID := range params.SpanIDs {
		pending[spanID] = struct{}{}
	}
	getTraceParams := GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{TraceID: params.TraceID}},
		RawTraces: false,
	}

	// AggregateTraces makes each ptrace.Traces hold a complete trace, not a chunk of one.
	result := SpanLookupResult{}
	for trace, err := range jptrace.AggregateTraces(qs.GetTraces(ctx, getTraceParams)) {
		if err != nil {
			return SpanLookupResult{}, fmt.Errorf("failed to get trace: %w", err)
		}
		result.TraceFound = true
		for pos, span := range jptrace.SpanIter(trace) {
			if _, found := pending[span.SpanID()]; found {
				result.Spans = append(result.Spans, SpanMatch{Resource: pos.Resource.Resource(), Span: span})
				delete(pending, span.SpanID())
			}
		}
	}
	return result, nil
}

// buildIdentityFilter names the requested spans as a predicate (RFC 0016 §4.3): trace ID and span
// ID are intrinsic span fields, so naming a span is comparing two of its own fields. Every
// requested span shares the one trace ID, so this is a single conjunction rather than the
// OR-of-ANDs a filter naming spans across several traces would need.
func buildIdentityFilter(traceID pcommon.TraceID, spanIDs []string) *expression.Call {
	return &expression.Call{
		Op: expression.OpAnd,
		Args: []expression.Expression{
			&expression.Call{
				Op: expression.OpEq,
				Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldTraceID},
					&expression.StringValue{Value: traceID.String()},
				},
			},
			&expression.Call{
				Op: expression.OpIn,
				Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldSpanID},
					&expression.List{Type: expression.ValueTypeString, Values: spanIDs},
				},
			},
		},
	}
}
