package adjuster

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	warningMaxTraceSize = "trace reached the maxium allowed size"
)

// CorrectMaxSize returns an Adjuster that validates if a trace is in the allowed max size
//
// foo 
//
// Parameters:
//   - maxSize: The maximum allowable trace size.
func CorrectMaxSize(maxTraceSize int) Adjuster {
	return Func(func(traces ptrace.Traces) {
	})
}
