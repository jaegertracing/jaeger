package nlquery

import (
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// Parse converts a natural language query into TraceQueryParams.
// This is a deterministic, rule-based baseline before LLM integration.
func Parse(input string) (Result, error) {
	q := strings.ToLower(input)

	params := tracestore.TraceQueryParams{}

	if strings.Contains(q, "error") || strings.Contains(q, "errors") {
		attrs := pcommon.NewMap()
		attrs.PutStr("error", "true")
		params.Attributes = attrs
	}

	return Result{
		Params: params,
		Reason: "rule-based fallback",
	}, nil
}
