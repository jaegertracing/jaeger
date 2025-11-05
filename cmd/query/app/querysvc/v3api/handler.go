package v3api

import (
	"context"
	"fmt"
	"time"

	api_v3 "github.com/jaegertracing/jaeger-idl/proto-gen/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Handler implements api_v3.QueryServiceServer
type Handler struct {
	api_v3.UnimplementedQueryServiceServer
	
	depReader dependencystore.Reader
	logger    *zap.Logger
}

// HandlerOptions contains options for creating a new Handler
type HandlerOptions struct {
	DependencyReader dependencystore.Reader
	Logger           *zap.Logger
}

// NewHandler creates a new v3 API handler
func NewHandler(options HandlerOptions) *Handler {
	return &Handler{
		depReader: options.DependencyReader,
		logger:    options.Logger,
	}
}

// GetDependencies retrieves service dependency links for a given time interval
func (h *Handler) GetDependencies(
	ctx context.Context,
	req *api_v3.GetDependenciesRequest,
) (*api_v3.GetDependenciesResponse, error) {
	// Validate request
	if err := h.validateGetDependenciesRequest(req); err != nil {
		h.logger.Error("Invalid GetDependencies request", zap.Error(err))
		return nil, err
	}

	endTime := req.EndTime.AsTime()
	lookback := req.Lookback.AsDuration()

	h.logger.Debug("Fetching dependencies",
		zap.Time("end_time", endTime),
		zap.Duration("lookback", lookback),
	)

	// Fetch dependencies from storage
	dependencies, err := h.depReader.GetDependencies(ctx, endTime, lookback)
	if err != nil {
		h.logger.Error("Failed to fetch dependencies from storage", zap.Error(err))
		return nil, status.Error(
			codes.Internal,
			fmt.Sprintf("failed to fetch dependencies: %v", err),
		)
	}

	// Convert model.DependencyLink to api_v3.DependencyLink
	links := make([]*api_v3.DependencyLink, 0, len(dependencies))
	for _, dep := range dependencies {
		links = append(links, &api_v3.DependencyLink{
			Parent:    dep.Parent,
			Child:     dep.Child,
			CallCount: dep.CallCount,
			Source:    dep.Source,
		})
	}

	h.logger.Debug("Successfully fetched dependencies",
		zap.Int("count", len(links)),
	)

	return &api_v3.GetDependenciesResponse{
		Dependencies: links,
	}, nil
}

// validateGetDependenciesRequest validates the GetDependencies request parameters
func (h *Handler) validateGetDependenciesRequest(req *api_v3.GetDependenciesRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if req.EndTime == nil {
		return status.Error(codes.InvalidArgument, "end_time is required")
	}

	if req.Lookback == nil {
		return status.Error(codes.InvalidArgument, "lookback is required")
	}

	endTime := req.EndTime.AsTime()
	lookback := req.Lookback.AsDuration()

	// Validate lookback is positive
	if lookback <= 0 {
		return status.Error(
			codes.InvalidArgument,
			"lookback must be a positive duration",
		)
	}

	// Validate end_time is not too far in the future (allow small clock skew)
	maxFutureTime := time.Now().Add(1 * time.Hour)
	if endTime.After(maxFutureTime) {
		return status.Error(
			codes.InvalidArgument,
			"end_time cannot be more than 1 hour in the future",
		)
	}

	// Validate the time range is reasonable (not too far back)
	startTime := endTime.Add(-lookback)
	minTime := time.Unix(0, 0)
	if startTime.Before(minTime) {
		return status.Error(
			codes.InvalidArgument,
			"time range extends before Unix epoch",
		)
	}

	return nil
}
