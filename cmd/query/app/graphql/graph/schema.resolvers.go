package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/query/app/graphql/graph/generated"
	"github.com/jaegertracing/jaeger/cmd/query/app/graphql/graph/model"
	v11 "github.com/jaegertracing/jaeger/pkg/otel/resource/v1"
	"github.com/jaegertracing/jaeger/pkg/otel/trace/v1"
)

func (r *queryResolver) Services(ctx context.Context) ([]string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Operations(ctx context.Context, service string) ([]string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Traces(ctx context.Context, service string, operationName *string, tags []string, minSpanDuration *int, maxSpanDuration *int, limit *int, startMicros int, endMicros int) ([]*v1.Span, error) {
	// TODO access the storage
	return nil, nil
}

func (r *queryResolver) Trace(ctx context.Context, traceID string) (*model.TracesResponse, error) {
	preloads := GetPreloads(ctx)
	names := map[string]bool{}
	for _, p := range preloads {
		names[p] = true
	}

	s := &v1.Span{
		TraceId: []byte{0, 1, 2, 3, 4},
		Name:    "hello",
	}

	// Remove fields that hasn't been requested
	// this is a workaround because:
	// * a custom marshaller is used for OTLP Span
	//   to ensure compatibility with OTLP JSON (e.g. standard JSON marshalling cannot be used for protos)
	// * a custom marshaller works well only with scalar types
	if !names["resourceSpans.instrumentationLibrarySpans.spans.traceId"] {
		s.TraceId = nil
	}
	if !names["resourceSpans.instrumentationLibrarySpans.spans.name"] {
		s.Name = ""
	}
	return &model.TracesResponse{
		ResourceSpans: &v1.ResourceSpans{
			InstrumentationLibrarySpans: []*v1.InstrumentationLibrarySpans{
				{
					Spans: []*v1.Span{s},
				},
			},
		},
	}, nil
}

func (r *resourceResolver) DroppedAttributesCount(ctx context.Context, obj *v11.Resource) (*int, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *spanResolver) SpanID(ctx context.Context, obj *v1.Span) (string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *spanResolver) TraceID(ctx context.Context, obj *v1.Span) (string, error) {
	panic(fmt.Errorf("not implemented"))
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Resource returns generated.ResourceResolver implementation.
func (r *Resolver) Resource() generated.ResourceResolver { return &resourceResolver{r} }

// Span returns generated.SpanResolver implementation.
func (r *Resolver) Span() generated.SpanResolver { return &spanResolver{r} }

type queryResolver struct{ *Resolver }
type resourceResolver struct{ *Resolver }
type spanResolver struct{ *Resolver }
