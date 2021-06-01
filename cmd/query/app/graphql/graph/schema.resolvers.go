package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"github.com/99designs/gqlgen/graphql"
	"math/rand"

	"github.com/jaegertracing/jaeger/cmd/query/app/graphql/graph/generated"
	"github.com/jaegertracing/jaeger/cmd/query/app/graphql/graph/model"
	"github.com/jaegertracing/jaeger/pkg/otel/trace/v1"
)

func (r *mutationResolver) CreateTodo(ctx context.Context, input model.NewTodo) (*model.Todo, error) {
	todo := &model.Todo{
		Text:   input.Text,
		ID:     fmt.Sprintf("T%d", rand.Int()),
		UserID: input.UserID, // fix this line
	}
	r.todos = append(r.todos, todo)
	return todo, nil
}

func (r *queryResolver) Todos(ctx context.Context) ([]*model.Todo, error) {
	fmt.Println("getting todos")
	return r.todos, nil
}

func (r *queryResolver) Services(ctx context.Context) ([]string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Operations(ctx context.Context, service string) ([]string, error) {
	panic(fmt.Errorf("not implemented"))
}

func (r *queryResolver) Traces(ctx context.Context, service string, operationName *string, minSpanDuration *int, maxSpanDuration *int, limit *int, startMicros int, endMicros int) ([]*v1.Span, error) {
	fmt.Println(*limit)
	fmt.Println(service)
	fmt.Println(operationName)
	return nil, nil
}

func (r *queryResolver) Trace(ctx context.Context, traceID string) ([]*v1.Span, error) {
	preloads := GetPreloads(ctx)
	names := map[string]bool{}
	for _, p := range preloads {
		names[p] = true
	}

	fmt.Println("Getting trace")
	fmt.Println(traceID)
	fmt.Println(preloads)
	s := &v1.Span{
		TraceId: []byte{0, 1, 2, 3, 4},
		Name: "mock name",
	}

	// Remove fields that hasn't been requested
	// this is a workaround because:
	// * a custom marshaller is used for OTLP Span
	//   to ensure compatibility with OTLP JSON (e.g. standard JSON marshalling cannot be used for protos)
	// * a custom marshaller works well only with scalar types
	if !names["traceId"] {
		s.TraceId = nil
	}
	if !names["name"] {
		s.Name = ""
	}
	return []*v1.Span{s}, nil
}

// These are virtual methods that do not map to a field in the model

func (r *spanResolver) SpanID(ctx context.Context, obj *v1.Span) (string, error) {
	fmt.Println("spanID")
	panic(fmt.Errorf("not implemented"))
}

func (r *spanResolver) TraceID(ctx context.Context, obj *v1.Span) (string, error) {
	fmt.Println("TraceID")
	panic(fmt.Errorf("not implemented"))
}

func (r *spanResolver) StartTimeUnixNano(ctx context.Context, obj *v1.Span) (*int, error) {
	fmt.Println("StartTimeUnixNano")
	panic(fmt.Errorf("not implemented"))
}

func (r *todoResolver) User(ctx context.Context, obj *model.Todo) (*model.User, error) {
	return &model.User{ID: obj.UserID, Name: "user " + obj.UserID}, nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Span returns generated.SpanResolver implementation.
func (r *Resolver) Span() generated.SpanResolver { return &spanResolver{r} }

// Todo returns generated.TodoResolver implementation.
func (r *Resolver) Todo() generated.TodoResolver { return &todoResolver{r} }

func GetPreloads(ctx context.Context) []string {
	return GetNestedPreloads(
		graphql.GetOperationContext(ctx),
		graphql.CollectFieldsCtx(ctx, nil),
		"",
	)
}

func GetNestedPreloads(ctx *graphql.OperationContext, fields []graphql.CollectedField, prefix string) (preloads []string) {
	for _, column := range fields {
		prefixColumn := GetPreloadString(prefix, column.Name)
		preloads = append(preloads, prefixColumn)
		preloads = append(preloads, GetNestedPreloads(ctx, graphql.CollectFields(ctx, column.Selections, nil), prefixColumn)...)
	}
	return
}

func GetPreloadString(prefix, name string) string {
	if len(prefix) > 0 {
		return prefix + "." + name
	}
	return name
}


type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type spanResolver struct{ *Resolver }
type todoResolver struct{ *Resolver }
