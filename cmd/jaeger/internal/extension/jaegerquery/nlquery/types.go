package nlquery

import "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"

//Result is the deterministic output of natural language parsing.
//The LLM is contrained to produce only this structure.

type Result struct {
	Params tracestore.TraceQueryParams
	Reason string // optional explaination for debugging/UI
}