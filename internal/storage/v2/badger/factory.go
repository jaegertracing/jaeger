// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"iter"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

const errorAttribute = "error"

type Factory struct {
	v1Factory *badger.Factory
}

func NewFactory(
	cfg badger.Config,
	telset telemetry.Settings,
) (*Factory, error) {
	v1Factory := badger.NewFactory()
	v1Factory.Config = &cfg
	err := v1Factory.Initialize(telset.Metrics, telset.Logger)
	if err != nil {
		return nil, err
	}
	f := Factory{v1Factory: v1Factory}
	return &f, nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	v1Writer, _ := f.v1Factory.CreateSpanWriter() // error is always nil
	return v1adapter.NewTraceWriter(v1Writer), nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	v1Reader, _ := f.v1Factory.CreateSpanReader() // error is always nil
	adaptedReader := v1adapter.NewTraceReader(v1Reader)
	return &traceReader{Reader: adaptedReader}, nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	v1Reader, _ := f.v1Factory.CreateDependencyReader() // error is always nil
	return v1adapter.NewDependencyReader(v1Reader), nil
}

func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return f.v1Factory.CreateSamplingStore(maxBuckets)
}

func (f *Factory) CreateLock() (distributedlock.Lock, error) {
	return f.v1Factory.CreateLock()
}

func (f *Factory) Close() error {
	return f.v1Factory.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	return f.v1Factory.Purge(ctx)
}

// traceReader wraps the v1adapter reader to provide v2-specific post-filtering
type traceReader struct {
	tracestore.Reader
}

// FindTraces implements post-filtering for v2 attribute queries
func (tr *traceReader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	// If no attributes to filter, delegate directly to v1 adapter
	if query.Attributes.Len() == 0 {
		return tr.Reader.FindTraces(ctx, query)
	}

	// Call v1 adapter first, then apply post-filtering
	return func(yield func([]ptrace.Traces, error) bool) {
		for traces, err := range tr.Reader.FindTraces(ctx, query) {
			if err != nil {
				yield(nil, err)
				return
			}

			// Filter traces based on v2 query parameters
			var filteredTraces []ptrace.Traces
			for _, td := range traces {
				if validTrace(td, query) {
					filteredTraces = append(filteredTraces, td)
				}
			}

			if len(filteredTraces) > 0 {
				if !yield(filteredTraces, nil) {
					return
				}
			}
		}
	}
}

// validTrace checks if a trace contains at least one span matching all query criteria
func validTrace(td ptrace.Traces, query tracestore.TraceQueryParams) bool {
	for _, resourceSpan := range td.ResourceSpans().All() {
		if !validResource(resourceSpan.Resource(), query) {
			continue
		}
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if validSpan(resourceSpan.Resource().Attributes(), scopeSpan.Scope(), span, query) {
					return true
				}
			}
		}
	}
	return false
}

// validResource checks if resource matches service name filter
func validResource(resource pcommon.Resource, query tracestore.TraceQueryParams) bool {
	return query.ServiceName == "" || query.ServiceName == getServiceNameFromResource(resource)
}

// validSpan checks if a span matches all query criteria
func validSpan(resourceAttributes pcommon.Map, scope pcommon.InstrumentationScope, span ptrace.Span, query tracestore.TraceQueryParams) bool {
	if query.OperationName != "" && query.OperationName != span.Name() {
		return false
	}

	startTime := span.StartTimestamp().AsTime()
	if !query.StartTimeMin.IsZero() && startTime.Before(query.StartTimeMin) {
		return false
	}
	if !query.StartTimeMax.IsZero() && startTime.After(query.StartTimeMax) {
		return false
	}
	duration := span.EndTimestamp().AsTime().Sub(startTime)
	if query.DurationMin != 0 && duration < query.DurationMin {
		return false
	}
	if query.DurationMax != 0 && duration > query.DurationMax {
		return false
	}

	// FIXED: Correct error flag handling
	if errAttribute, ok := query.Attributes.Get(errorAttribute); ok {
		if errAttribute.Bool() && span.Status().Code() != ptrace.StatusCodeError {
			return false
		}
		// FIXED: Accept both StatusCodeOk AND StatusCodeUnset for error=false
		if !errAttribute.Bool() && span.Status().Code() == ptrace.StatusCodeError {
			return false
		}
	}

	if statusAttr, ok := query.Attributes.Get("span.status"); ok {
		expectedStatus := spanStatusFromString(statusAttr.AsString())
		if expectedStatus != span.Status().Code() {
			return false
		}
	}

	if kindAttr, ok := query.Attributes.Get("span.kind"); ok {
		expectedKind := spanKindFromString(kindAttr.AsString())
		if expectedKind != span.Kind() {
			return false
		}
	}

	if scopeNameAttr, ok := query.Attributes.Get("scope.name"); ok {
		if scopeNameAttr.AsString() != scope.Name() {
			return false
		}
	}

	if scopeVersionAttr, ok := query.Attributes.Get("scope.version"); ok {
		if scopeVersionAttr.AsString() != scope.Version() {
			return false
		}
	}

	for key, val := range query.Attributes.All() {
		if key == errorAttribute ||
			key == "span.status" ||
			key == "span.kind" ||
			key == "scope.name" ||
			key == "scope.version" {
			continue
		}

		if strings.HasPrefix(key, "resource.") {
			resourceKey := strings.TrimPrefix(key, "resource.")
			if !matchAttributes(resourceKey, val, resourceAttributes) {
				return false
			}
			continue
		}

		if !findKeyValInTrace(key, val, resourceAttributes, scope.Attributes(), span) {
			return false
		}
	}

	return true
}

// matchAttributes performs type-safe attribute comparison
func matchAttributes(key string, val pcommon.Value, attrs pcommon.Map) bool {
	if queryValue, ok := attrs.Get(key); ok {
		// FIXED: Use type-safe comparison instead of string comparison
		return queryValue.Equal(val)
	}
	return false
}

// findKeyValInTrace searches for key-value pair in span, scope, resource attributes, and events
func findKeyValInTrace(key string, val pcommon.Value, resourceAttributes pcommon.Map, scopeAttributes pcommon.Map, span ptrace.Span) bool {
	tagsMatched := matchAttributes(key, val, span.Attributes()) || matchAttributes(key, val, scopeAttributes) || matchAttributes(key, val, resourceAttributes)
	if tagsMatched {
		return true
	}
	for _, event := range span.Events().All() {
		if matchAttributes(key, val, event.Attributes()) {
			return true
		}
	}
	return tagsMatched
}

// getServiceNameFromResource extracts service name from resource attributes
func getServiceNameFromResource(resource pcommon.Resource) string {
	val, ok := resource.Attributes().Get(conventions.ServiceNameKey)
	if !ok {
		return ""
	}
	return val.Str()
}

// spanStatusFromString converts string to ptrace.StatusCode
func spanStatusFromString(statusStr string) ptrace.StatusCode {
	switch strings.ToUpper(statusStr) {
	case "OK":
		return ptrace.StatusCodeOk
	case "ERROR":
		return ptrace.StatusCodeError
	default:
		return ptrace.StatusCodeUnset
	}
}

// spanKindFromString converts string to ptrace.SpanKind
func spanKindFromString(kindStr string) ptrace.SpanKind {
	switch strings.ToUpper(kindStr) {
	case "CLIENT":
		return ptrace.SpanKindClient
	case "SERVER":
		return ptrace.SpanKindServer
	case "PRODUCER":
		return ptrace.SpanKindProducer
	case "CONSUMER":
		return ptrace.SpanKindConsumer
	case "INTERNAL":
		return ptrace.SpanKindInternal
	default:
		return ptrace.SpanKindUnspecified
	}
}
