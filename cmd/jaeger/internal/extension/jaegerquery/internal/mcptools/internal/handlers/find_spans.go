// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// findSpansQueryService defines the interface we need from QueryService for find_spans.
type findSpansQueryService interface {
	FindSpans(ctx context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error]
}

// findSpansHandler implements the find_spans MCP tool (RFC 0016 M7). Unlike
// get_span_details, which names spans it already knows the IDs of, this tool discovers
// spans matching service/name/attribute/duration/error criteria across however many
// traces in one call, which today takes a search_traces call followed by one or more
// get_span_details calls.
type findSpansHandler struct {
	queryService findSpansQueryService
	maxResults   int
}

// NewFindSpansHandler creates a new find_spans handler and returns the handler function.
func NewFindSpansHandler(
	queryService *querysvc.QueryService,
	maxResults int,
) mcp.ToolHandlerFor[types.FindSpansInput, types.FindSpansOutput] {
	h := &findSpansHandler{
		queryService: queryService,
		maxResults:   maxResults,
	}
	return h.handle
}

// handle processes the find_spans tool request.
func (h *findSpansHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.FindSpansInput,
) (*mcp.CallToolResult, types.FindSpansOutput, error) {
	query, limit, err := h.buildQuery(input)
	if err != nil {
		return nil, types.FindSpansOutput{}, err
	}

	var spans []types.SpanDetail
	var processErrs []error

outer:
	for chunk, err := range h.queryService.FindSpans(ctx, query) {
		if err != nil {
			processErrs = append(processErrs, err)
			break
		}
		for pos, span := range jptrace.SpanIter(chunk.Results) {
			spans = append(spans, buildSpanDetail(pos, span))
			if limit > 0 && len(spans) >= limit {
				break outer
			}
		}
	}

	output := types.FindSpansOutput{Spans: spans}
	if len(processErrs) > 0 {
		searchErr := errors.Join(processErrs...)
		if len(spans) == 0 {
			// Nothing came back, so calling it partial would send an agent looking for
			// the missing part instead of reading why the search could not run: a
			// backend refusing the query (ErrSpanSearchUnsupported, ErrFilterDisabled)
			// reports the same way search_traces reports FindTraceSummaries refusing.
			output.Error = searchErr.Error()
		} else {
			output.Error = fmt.Sprintf("partial results returned due to error: %v", searchErr)
		}
	}
	return nil, output, nil
}

// buildQuery converts FindSpansInput to querysvc.SpanQueryParams and the effective
// result limit (input.MaxResults, clamped to the server's configured cap).
func (h *findSpansHandler) buildQuery(input types.FindSpansInput) (querysvc.SpanQueryParams, int, error) {
	if input.StartTimeMin == "" {
		return querysvc.SpanQueryParams{}, 0, errors.New("start_time_min is required")
	}
	if input.StartTimeMax == "" {
		return querysvc.SpanQueryParams{}, 0, errors.New("start_time_max is required")
	}
	startTimeMin, err := parseTimeParam(input.StartTimeMin)
	if err != nil {
		return querysvc.SpanQueryParams{}, 0, fmt.Errorf("invalid start_time_min: %w", err)
	}
	startTimeMax, err := parseTimeParam(input.StartTimeMax)
	if err != nil {
		return querysvc.SpanQueryParams{}, 0, fmt.Errorf("invalid start_time_max: %w", err)
	}

	var durationMin, durationMax time.Duration
	if input.DurationMin != "" {
		durationMin, err = time.ParseDuration(input.DurationMin)
		if err != nil {
			return querysvc.SpanQueryParams{}, 0, fmt.Errorf("invalid duration_min: %w", err)
		}
	}
	if input.DurationMax != "" {
		durationMax, err = time.ParseDuration(input.DurationMax)
		if err != nil {
			return querysvc.SpanQueryParams{}, 0, fmt.Errorf("invalid duration_max: %w", err)
		}
	}

	limit := input.MaxResults
	if limit <= 0 || (h.maxResults > 0 && limit > h.maxResults) {
		limit = h.maxResults
	}

	return querysvc.SpanQueryParams{SpanQueryParams: tracestore.SpanQueryParams{
		StartTimeMin: startTimeMin,
		StartTimeMax: startTimeMax,
		Filter:       buildSearchFilter(input, durationMin, durationMax),
	}}, limit, nil
}

// buildSearchFilter translates the tool's flat, LLM-friendly parameters into the RFC 0005
// filter FindSpans actually takes, rather than exposing the filter AST to the model
// directly. Every present criterion becomes one predicate, ANDed together; no criteria
// leaves the filter nil, which is the base case of a span query: the time range alone
// (RFC 0016 §5.1).
//
// An attribute value from the input map is passed as an untyped AnyValue rather than a
// StringValue, so it resolves against whatever kind the attribute is actually stored as
// (string, number or bool) instead of only ever matching a literal string (RFC 0005 §5.4).
// with_errors reads as the error virtual attribute every backend's filter lowering
// special-cases (e.g. the elasticsearch backend's asErrorTagEquality), not a stored
// attribute of that name.
func buildSearchFilter(input types.FindSpansInput, durationMin, durationMax time.Duration) *expression.Call {
	var predicates []expression.Expression

	if input.ServiceName != "" {
		predicates = append(predicates, eq(
			&expression.FieldRef{Level: expression.LevelResource, Name: expression.ResourceFieldService},
			&expression.StringValue{Value: input.ServiceName},
		))
	}
	if input.SpanName != "" {
		predicates = append(predicates, eq(
			&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
			&expression.StringValue{Value: input.SpanName},
		))
	}
	for key, value := range input.Attributes {
		predicates = append(predicates, eq(
			&expression.AttributeRef{Key: key},
			&expression.AnyValue{Value: value},
		))
	}
	if input.WithErrors {
		predicates = append(predicates, eq(
			&expression.AttributeRef{Key: "error"},
			&expression.BoolValue{Value: true},
		))
	}
	if durationMin != 0 {
		predicates = append(predicates, &expression.Call{
			Op: expression.OpGte,
			Args: []expression.Expression{
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
				&expression.DurationValue{Value: durationMin},
			},
		})
	}
	if durationMax != 0 {
		predicates = append(predicates, &expression.Call{
			Op: expression.OpLte,
			Args: []expression.Expression{
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
				&expression.DurationValue{Value: durationMax},
			},
		})
	}

	switch len(predicates) {
	case 0:
		return nil
	case 1:
		return predicates[0].(*expression.Call)
	default:
		return &expression.Call{Op: expression.OpAnd, Args: predicates}
	}
}

func eq(ref, value expression.Expression) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{ref, value}}
}
