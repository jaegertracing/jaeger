// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

// TODO: the JSON types below are temporary scaffolding until the FindTraceSummaries
// RPC is added to jaeger-idl and proto-generated types replace them (ADR-010, Milestone 3).

import (
	"encoding/json"
	"net/http"

	"github.com/jaegertracing/jaeger/internal/jiter"
)

// serviceSummaryJSON is the JSON representation of a per-service summary.
type serviceSummaryJSON struct {
	Name           string `json:"name"`
	SpanCount      int    `json:"spanCount"`
	ErrorSpanCount int    `json:"errorSpanCount"`
}

// traceSummaryJSON is the JSON representation of a trace summary.
type traceSummaryJSON struct {
	TraceID           string               `json:"traceID"`
	RootServiceName   string               `json:"rootServiceName"`
	RootOperationName string               `json:"rootOperationName"`
	StartTimeUnixUs   int64                `json:"startTimeUnixUs"`
	DurationUs        int64                `json:"durationUs"`
	SpanCount         int                  `json:"spanCount"`
	ErrorSpanCount    int                  `json:"errorSpanCount"`
	OrphanSpanCount   int                  `json:"orphanSpanCount"`
	Services          []serviceSummaryJSON `json:"services"`
}

// findTraceSummariesResponseJSON is the JSON envelope for the FindTraceSummaries response.
type findTraceSummariesResponseJSON struct {
	Summaries []traceSummaryJSON `json:"summaries"`
}

func (h *HTTPGateway) findTraceSummaries(w http.ResponseWriter, r *http.Request) {
	queryParams, shouldReturn := h.parseFindTracesQuery(r.URL.Query(), w)
	if shouldReturn {
		return
	}
	summariesIter := h.QueryService.FindTraceSummaries(r.Context(), *queryParams)
	summaries, err := jiter.FlattenWithErrors(summariesIter)
	if h.tryHandleError(w, err, http.StatusInternalServerError) {
		return
	}
	response := findTraceSummariesResponseJSON{
		Summaries: make([]traceSummaryJSON, len(summaries)),
	}
	for i := range summaries {
		s := &summaries[i]
		svcJSON := make([]serviceSummaryJSON, len(s.Services))
		for j, svc := range s.Services {
			svcJSON[j] = serviceSummaryJSON{
				Name:           svc.Name,
				SpanCount:      svc.SpanCount,
				ErrorSpanCount: svc.ErrorSpanCount,
			}
		}
		response.Summaries[i] = traceSummaryJSON{
			TraceID:           s.TraceID.String(),
			RootServiceName:   s.RootServiceName,
			RootOperationName: s.RootOperationName,
			StartTimeUnixUs:   s.StartTime.UnixMicro(),
			DurationUs:        s.Duration.Microseconds(),
			SpanCount:         s.SpanCount,
			ErrorSpanCount:    s.ErrorSpanCount,
			OrphanSpanCount:   s.OrphanSpanCount,
			Services:          svcJSON,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
