// Copyright (c) 2023 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tracing

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel/baggage"
)

func BaggageItem(ctx context.Context, key string) string {
	val := opentracingBaggageItem(ctx, key)
	if val != "" {
		return val
	}
	return otelBaggageItem(ctx, key)
}

func opentracingBaggageItem(ctx context.Context, key string) string {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	return span.BaggageItem(key)
}

func otelBaggageItem(ctx context.Context, key string) string {
	b := baggage.FromContext(ctx)
	m := b.Member(key)
	return m.Value()
}
