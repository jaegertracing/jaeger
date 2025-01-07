package sanitizer

import (
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/sanitizer"
)

// Sanitize is a function that applies all sanitizers to the given trace data.
var Sanitize = sanitizer.NewChainedSanitizer(sanitizer.NewStandardSanitizers()...)
