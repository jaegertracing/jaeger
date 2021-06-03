package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/jaegertracing/jaeger/cmd/query/app/gqlgen/graph"
	"github.com/jaegertracing/jaeger/cmd/query/app/gqlgen/graph/generated"
)

const defaultPort = "8080"

// millis 1622041719349
// micros 1622041719349000
// ns     1622041719349000000
//        1621868040000000
func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	r := &graph.Resolver{
		//internalspans: []*model.span{
		//	{
		//		id: "one",
		//		traceid: "one",
		//		operationname: "foo",
		//	},
		//},
	}
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: r}))

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)

	http.ListenAndServe(":8080", nil)
}
