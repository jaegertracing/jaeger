# Implementation Details: Removing v1 QueryService

This document provides detailed technical guidance for implementing the v1 QueryService removal as described in the investigation report.

## Current Code Structure

### Files Involved

#### Core Files
- `cmd/jaeger/internal/extension/jaegerquery/server.go` - Creates both QueryServices
- `cmd/jaeger/internal/extension/jaegerquery/internal/server.go` - Server initialization
- `cmd/jaeger/internal/extension/jaegerquery/internal/grpc_handler.go` - API v2 gRPC handler
- `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go` - API v2 HTTP handler

#### QueryService Implementations
- `cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/query_service.go` - v1 QueryService
- `cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/v2/querysvc/service.go` - v2 QueryService

#### API v3 Handlers (Already using v2)
- `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/grpc_handler.go`
- `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/http_gateway.go`

#### Storage Adapters
- `internal/storage/v2/v1adapter/spanreader.go` - Converts v2 → v1 storage API

## Detailed Implementation Steps

### Step 1: Understand Existing Conversion Utilities

The `internal/storage/v2/v1adapter` package already contains the conversion utilities we need:

- `V1TracesFromSeq2(iter.Seq2[[]ptrace.Traces, error]) ([]*model.Trace, error)` - Converts iterator to v1 traces
- `V1BatchesFromTraces(ptrace.Traces) []*model.Batch` - Converts ptrace to v1 batches
- Other helper functions for trace and span ID conversions

**No new converter package needed!** We'll use these existing utilities.

### Step 2: Update GRPCHandler

**File:** `cmd/jaeger/internal/extension/jaegerquery/internal/grpc_handler.go`

#### Changes to GRPCHandler struct:
```go
type GRPCHandler struct {
    queryService *v2querysvc.QueryService  // Changed from querysvc.QueryService
    logger       *zap.Logger
    nowFn        func() time.Time
}
```

#### Update Constructor:
```go
func NewGRPCHandler(
    queryService *v2querysvc.QueryService,  // Changed from querysvc.QueryService
    options GRPCHandlerOptions,
) *GRPCHandler {
    // ... rest unchanged
}
```

#### Update GetTrace Method:
```go
func (g *GRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
    if r == nil {
        return errNilRequest
    }
    if r.TraceID == (model.TraceID{}) {
        return errUninitializedTraceID
    }
    
    // Convert to v2 query parameters
    query := v2querysvc.GetTraceParams{
        TraceIDs: []tracestore.GetTraceParams{
            {
                TraceID:   v1adapter.FromV1TraceID(r.TraceID),
                Start:     r.StartTime,
                End:       r.EndTime,
            },
        },
        RawTraces: r.RawTraces,
    }
    
    // Get traces using v2 QueryService
    getTracesIter := g.queryService.GetTraces(stream.Context(), query)
    
    // Convert iterator to v1 traces using existing v1adapter utility
    traces, err := v1adapter.V1TracesFromSeq2(getTracesIter)
    if err != nil {
        g.logger.Error("failed to retrieve or convert traces", zap.Error(err))
        return status.Errorf(codes.Internal, "failed to retrieve or convert traces: %v", err)
    }
    if len(traces) == 0 {
        g.logger.Warn(msgTraceNotFound, zap.Stringer("id", r.TraceID))
        return status.Errorf(codes.NotFound, "%s", msgTraceNotFound)
    }
    
    // Send the first trace (GetTrace should return single trace)
    return g.sendSpanChunks(traces[0].Spans, stream.Send)
}
```

#### Update FindTraces Method:
```go
func (g *GRPCHandler) FindTraces(r *api_v2.FindTracesRequest, stream api_v2.QueryService_FindTracesServer) error {
    if r == nil {
        return errNilRequest
    }
    query := r.GetQuery()
    if query == nil {
        return status.Errorf(codes.InvalidArgument, "missing query")
    }
    
    // Convert to v2 query parameters
    queryParams := v2querysvc.TraceQueryParams{
        TraceQueryParams: tracestore.TraceQueryParams{
            ServiceName:   query.ServiceName,
            OperationName: query.OperationName,
            // Convert tags to attributes
            Attributes:    convertTagsToAttributes(query.Tags),
            StartTimeMin:  query.StartTimeMin,
            StartTimeMax:  query.StartTimeMax,
            DurationMin:   query.DurationMin,
            DurationMax:   query.DurationMax,
            SearchDepth:   int(query.SearchDepth),
        },
        RawTraces: query.RawTraces,
    }
    
    // Find traces using v2 QueryService
    findTracesIter := g.queryService.FindTraces(stream.Context(), queryParams)
    
    // Convert and stream traces using existing v1adapter utility
    traces, err := v1adapter.V1TracesFromSeq2(findTracesIter)
    if err != nil {
        g.logger.Error("failed when searching for traces", zap.Error(err))
        return status.Errorf(codes.Internal, "failed when searching for traces: %v", err)
    }
    
    for _, trace := range traces {
        if err := g.sendSpanChunks(trace.Spans, stream.Send); err != nil {
            return err
        }
    }
    return nil
}
```

#### Update ArchiveTrace Method:
```go
func (g *GRPCHandler) ArchiveTrace(ctx context.Context, r *api_v2.ArchiveTraceRequest) (*api_v2.ArchiveTraceResponse, error) {
    if r == nil {
        return nil, errNilRequest
    }
    if r.TraceID == (model.TraceID{}) {
        return nil, errUninitializedTraceID
    }
    
    query := tracestore.GetTraceParams{
        TraceID: v1adapter.FromV1TraceID(r.TraceID),
        Start:   r.StartTime,
        End:     r.EndTime,
    }

    err := g.queryService.ArchiveTrace(ctx, query)
    if err != nil {
        g.logger.Error("failed to archive trace", zap.Error(err))
        return nil, status.Errorf(codes.Internal, "failed to archive trace: %v", err)
    }

    return &api_v2.ArchiveTraceResponse{}, nil
}
```

#### Add Helper Function for Tags Conversion:
```go
func convertTagsToAttributes(tags map[string]string) map[string]any {
    attrs := make(map[string]any, len(tags))
    for k, v := range tags {
        attrs[k] = v
    }
    return attrs
}
```

### Step 3: Update HTTP Handler (APIHandler)

**File:** `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go`

#### Update APIHandler struct:
```go
type APIHandler struct {
    queryService        *v2querysvc.QueryService  // Changed from querysvc.QueryService
    metricsQueryService querysvc.MetricsQueryService
    queryParser         queryParser
    basePath            string
    apiPrefix           string
    logger              *zap.Logger
    tracer              trace.TracerProvider
}
```

#### Update Constructor:
```go
func NewAPIHandler(
    queryService *v2querysvc.QueryService,  // Changed
    options ...HandlerOption,
) *APIHandler {
    // ... rest unchanged
}
```

#### Update trace query methods similarly to GRPCHandler
- Use v2 QueryService methods
- Convert results using v2adapter utilities
- Maintain exact same response format

### Step 4: Update Server Initialization

**File:** `cmd/jaeger/internal/extension/jaegerquery/server.go`

Remove v1 QueryService creation:

```go
func (s *server) Start(ctx context.Context, host component.Host) error {
    // ... existing code until traceReader and depReader creation ...
    
    // Remove these lines:
    // opts := querysvc.QueryServiceOptions{
    //     MaxClockSkewAdjust: s.config.MaxClockSkewAdjust,
    // }
    
    v2opts := v2querysvc.QueryServiceOptions{
        MaxClockSkewAdjust: s.config.MaxClockSkewAdjust,
    }
    
    // Remove this call: if err := s.addArchiveStorage(&opts, &v2opts, host)
    if err := s.addArchiveStorage(&v2opts, host); err != nil {
        return err
    }
    
    // Remove this line: qs := querysvc.NewQueryService(traceReader, depReader, opts)
    v2qs := v2querysvc.NewQueryService(traceReader, depReader, v2opts)

    // ... rest of function ...
    
    s.server, err = queryapp.NewServer(
        ctx,
        v2qs,  // Pass only v2 QueryService
        mqs,
        &s.config.QueryOptions,
        tm,
        telset,
    )
    // ... rest unchanged ...
}
```

Update `addArchiveStorage`:
```go
func (s *server) addArchiveStorage(
    v2opts *v2querysvc.QueryServiceOptions,
    host component.Host,
) error {
    if s.config.Storage.TracesArchive == "" {
        s.telset.Logger.Info("Archive storage not configured")
        return nil
    }

    f, err := jaegerstorage.GetTraceStoreFactory(s.config.Storage.TracesArchive, host)
    if err != nil {
        return fmt.Errorf("cannot find traces archive storage factory: %w", err)
    }

    traceReader, traceWriter := s.initArchiveStorage(f)
    if traceReader == nil || traceWriter == nil {
        return nil
    }

    v2opts.ArchiveTraceReader = traceReader
    v2opts.ArchiveTraceWriter = traceWriter

    return nil
}
```

**File:** `cmd/jaeger/internal/extension/jaegerquery/internal/server.go`

Update `NewServer` signature:
```go
func NewServer(
    ctx context.Context,
    v2QuerySvc *v2querysvc.QueryService,  // Changed parameter name and type
    metricsQuerySvc querysvc.MetricsQueryService,
    options *QueryOptions,
    tm *tenancy.Manager,
    telset telemetry.Settings,
) (*Server, error) {
    // ... validation code ...
    
    grpcServer, err := createGRPCServer(ctx, options, tm, telset)
    if err != nil {
        return nil, err
    }
    registerGRPCHandlers(grpcServer, v2QuerySvc, telset)  // Updated call
    
    httpServer, err := createHTTPServer(ctx, v2QuerySvc, metricsQuerySvc, options, tm, telset)
    if err != nil {
        return nil, err
    }

    return &Server{
        querySvc:     nil,  // TODO: Remove this field in a follow-up cleanup PR
                            // Setting to nil to avoid using v1 QueryService accidentally
        queryOptions: options,
        grpcServer:   grpcServer,
        httpServer:   httpServer,
        telset:       telset,
    }, nil
}
```

Update `registerGRPCHandlers`:
```go
func registerGRPCHandlers(
    server *grpc.Server,
    v2QuerySvc *v2querysvc.QueryService,  // Changed
    telset telemetry.Settings,
) {
    reflection.Register(server)
    handler := NewGRPCHandler(v2QuerySvc, GRPCHandlerOptions{Logger: telset.Logger})  // Updated
    healthServer := health.NewServer()

    api_v2.RegisterQueryServiceServer(server, handler)
    api_v3.RegisterQueryServiceServer(server, &apiv3.Handler{QueryService: v2QuerySvc})

    // ... rest unchanged ...
}
```

Update `createHTTPServer` and `initRouter`:
```go
func initRouter(
    v2QuerySvc *v2querysvc.QueryService,  // Changed
    metricsQuerySvc querysvc.MetricsQueryService,
    queryOpts *QueryOptions,
    tenancyMgr *tenancy.Manager,
    telset telemetry.Settings,
) (http.Handler, io.Closer) {
    apiHandlerOptions := []HandlerOption{
        HandlerOptions.Logger(telset.Logger),
        HandlerOptions.Tracer(telset.TracerProvider),
        HandlerOptions.MetricsQueryService(metricsQuerySvc),
    }

    apiHandler := NewAPIHandler(
        v2QuerySvc,  // Updated
        apiHandlerOptions...)
    // ... rest of function ...
    
    // Note: Need to handle GetCapabilities() - see below
    staticHandlerCloser := RegisterStaticHandler(r, telset.Logger, queryOpts, v2QuerySvc.GetCapabilities())
    
    // ... rest unchanged ...
}

func createHTTPServer(
    ctx context.Context,
    v2QuerySvc *v2querysvc.QueryService,  // Changed
    metricsQuerySvc querysvc.MetricsQueryService,
    queryOpts *QueryOptions,
    tm *tenancy.Manager,
    telset telemetry.Settings,
) (*httpServer, error) {
    handler, staticHandlerCloser := initRouter(v2QuerySvc, metricsQuerySvc, queryOpts, tm, telset)
    // ... rest unchanged ...
}
```

### Step 5: Handle Storage Capabilities

Both v1 and v2 QueryService have `GetCapabilities()` method that returns `StorageCapabilities` with the same structure. No changes needed here.

### Step 6: Update Tests

Update all tests that create handlers to use v2 QueryService instead of v1:

- `cmd/jaeger/internal/extension/jaegerquery/internal/grpc_handler_test.go`
- `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler_test.go`
- `cmd/jaeger/internal/extension/jaegerquery/internal/server_test.go`
- Any other tests that mock or create QueryService

## Testing Strategy

### Unit Tests
1. Test conversion utilities thoroughly
2. Update existing handler tests to use v2 QueryService
3. Verify exact same responses for API v2 endpoints

### Integration Tests
1. Run full API v2 gRPC test suite
2. Run full API v2 HTTP test suite
3. Verify archive storage functionality
4. Test error handling paths

### End-to-End Tests
1. Run existing E2E tests
2. Verify UI functionality (uses API v2)
3. Test with real storage backends

## Rollback Plan

If issues are discovered:
1. Revert the PR
2. The v1 QueryService implementation remains intact in the codebase
3. Analyze issues and refine conversion logic
4. Retry with improved implementation

## Performance Considerations

### Potential Improvements
- v2 QueryService uses iterators, enabling better streaming
- Direct v2 storage access may be more efficient

### Potential Concerns
- Additional conversion overhead from ptrace → v1 model
- Need to benchmark critical query paths

## Documentation Updates

Update the following documentation:
1. Architecture documentation about query service layers
2. API documentation (if any references v1 QueryService)
3. Developer guides about storage interfaces
4. Migration guide for external users (if applicable)

## Success Metrics

Before merging, verify:
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] E2E tests pass
- [ ] No performance regression (< 5% slowdown acceptable)
- [ ] Code coverage maintained or improved
- [ ] UI continues to function correctly
- [ ] Archive storage works as before
- [ ] Both gRPC and HTTP API v2 work correctly

## Follow-up Tasks (Future PRs)

After successful migration:
1. Mark v1 QueryService as deprecated
2. **Remove `Server.querySvc` field** - no longer needed after this change
3. Consider removing v1 QueryService entirely in next major version
4. Update storage layer documentation
5. Simplify storage adapter layer if possible
