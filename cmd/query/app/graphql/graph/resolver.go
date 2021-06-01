package graph

import "github.com/jaegertracing/jaeger/cmd/query/app/graphql/graph/model"

type Resolver struct{
	todos []*model.Todo
}
