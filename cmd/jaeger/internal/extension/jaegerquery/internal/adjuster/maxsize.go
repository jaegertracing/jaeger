// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"strconv"

	"github.com/docker/go-units"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

const (
	warningMaxTraceSize = "trace reached the maximum allowed size of %s bytes; trace size is %s bytes"
)

// CorrectMaxSize returns an Adjuster that validates if a trace is in the allowed max size
//
// This adjuster calculates the size of the trace and compares it to the specified maximum size.
//
// Parameters:
//   - maxSize: The maximum allowable trace size.
func CorrectMaxSize(maxTraceSize string) Adjuster {
	return Func(func(traces ptrace.Traces) {
		maxTraceSizeBytes, err := units.RAMInBytes(maxTraceSize)
		if err != nil {
			return
		}

		// no limit
		if maxTraceSizeBytes == 0 {
			return
		}

		marshaler := &ptrace.ProtoMarshaler{}
		traceSizeBytes := int64(marshaler.TracesSize(traces))
		if traceSizeBytes > maxTraceSizeBytes {
			// TODO: not sure if this is the right approach to handle big traces
			// should we drop the trace instead of adding warnings to all spans?
			// or should we add a warning to the root span only?
			resources := traces.ResourceSpans()
			for i := range resources.Len() {
				resource := resources.At(i)
				scopes := resource.ScopeSpans()
				for j := range scopes.Len() {
					spans := scopes.At(j).Spans()
					for k := range spans.Len() {
						span := spans.At(k)
						jptrace.AddWarnings(
							span,
							warningMaxTraceSize,
							strconv.FormatInt(maxTraceSizeBytes, 10),
							strconv.FormatInt(traceSizeBytes, 10),
						)
					}
				}
			}
		}
	})
}
