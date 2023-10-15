package adjuster

import (
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/jaegertracing/jaeger/model"
)

var otelLibraryKeys = map[string]struct{}{
	string(semconv.OTelLibraryNameKey):    {},
	string(semconv.OTelLibraryVersionKey): {},
}

func OTelTagAdjuster() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		for _, span := range trace.Spans {
			filteredTags := make([]model.KeyValue, 0)
			for _, tag := range span.Tags {
				if _, ok := otelLibraryKeys[tag.Key]; !ok {
					filteredTags = append(filteredTags, tag)
					continue
				}
				span.Process.Tags = append(span.Process.Tags, tag)
			}
			span.Tags = filteredTags
			model.KeyValues(span.Process.Tags).Sort()
		}
		return trace, nil
	})
}
