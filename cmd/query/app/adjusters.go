package app

import "github.com/uber/jaeger/model/adjuster"

// StandardAdjusters is a list of model adjusters applied by the query service
// before returning the data to the API clients.
var StandardAdjusters = []adjuster.Adjuster{
	adjuster.SpanIDDeduper(),
	adjuster.ClockSkew(),
	adjuster.IPTagAdjuster(),
	adjuster.SortLogFields(),
}
