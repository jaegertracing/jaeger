// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// CapabilitiesHandler implements api_v3.CapabilitiesServer. It answers from the trace reader's
// declaration on every call, the same way the query service enforces it, so the API reports
// exactly what a search is held to.
type CapabilitiesHandler struct {
	api_v3.UnimplementedCapabilitiesServer
	QueryService *querysvc.QueryService
}

var _ api_v3.CapabilitiesServer = (*CapabilitiesHandler)(nil)

// GetCapabilities implements api_v3.CapabilitiesServer's GetCapabilities. A reader that cannot
// determine its own capabilities is reported as UNIMPLEMENTED, which a client reads as the least
// capable backend rather than as a declaration that nothing is supported.
func (h *CapabilitiesHandler) GetCapabilities(ctx context.Context, _ *api_v3.GetCapabilitiesRequest) (*api_v3.GetCapabilitiesResponse, error) {
	caps, err := h.QueryService.SearchCapabilities(ctx)
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			return nil, status.Error(codes.Unimplemented, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &api_v3.GetCapabilitiesResponse{Search: toSearchCapabilities(caps)}, nil
}

// toSearchCapabilities converts the reader's declaration to its api_v3 shape, which mirrors
// jaeger.storage.v2.SearchCapabilities field for field.
func toSearchCapabilities(caps tracestore.SearchCapabilities) *api_v3.SearchCapabilities {
	return &api_v3.SearchCapabilities{
		WithoutServiceName:  caps.WithoutServiceName,
		SameSpanConjunction: caps.SameSpanConjunction,
		Filter:              toFilterCapabilities(caps.Filter),
		Paginated:           caps.Paginated,
		SpanSearch:          caps.SpanSearch,
		SpanSorting:         caps.SpanSorting,
	}
}

func toFilterCapabilities(caps *tracestore.FilterCapabilities) *api_v3.FilterCapabilities {
	if caps == nil {
		return nil
	}
	levels := make([]string, 0, len(caps.Levels))
	for _, level := range caps.Levels {
		levels = append(levels, string(level))
	}
	operators := make([]string, 0, len(caps.Operators))
	for _, op := range caps.Operators {
		operators = append(operators, string(op))
	}
	return &api_v3.FilterCapabilities{
		Levels:    levels,
		Operators: operators,
	}
}
