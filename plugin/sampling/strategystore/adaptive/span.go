// Copyright (c) 2021 The Jaeger Authors.
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

package adaptive

import (
	"strconv"

	"github.com/uber/jaeger-client-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

// GetSamplerParams returns the sampler.type and sampler.param value if they are valid.
func GetSamplerParams(span *model.Span, logger *zap.Logger) (string, float64) {
	// TODO move this into model.Span
	tag, ok := model.KeyValues(span.Tags).FindByKey(jaeger.SamplerTypeTagKey)
	if !ok {
		return "", 0
	}
	if tag.VType != model.StringType {
		logger.
			With(zap.String("traceID", span.TraceID.String())).
			With(zap.String("spanID", span.SpanID.String())).
			Warn("sampler.type tag is not a string", zap.Any("tag", tag))
		return "", 0
	}
	samplerType := tag.AsString()
	if samplerType != jaeger.SamplerTypeProbabilistic && samplerType != jaeger.SamplerTypeLowerBound &&
		samplerType != jaeger.SamplerTypeRateLimiting {
		return "", 0
	}
	tag, ok = model.KeyValues(span.Tags).FindByKey(jaeger.SamplerParamTagKey)
	if !ok {
		return "", 0
	}
	samplerParam, err := getParam(tag)
	if err != nil {
		logger.
			With(zap.String("traceID", span.TraceID.String())).
			With(zap.String("spanID", span.SpanID.String())).
			Warn("sampler.param tag is not a number", zap.Any("tag", tag))
		return "", 0
	}
	return samplerType, samplerParam
}

func getParam(samplerParamTag model.KeyValue) (float64, error) {
	// The param could be represented as a string, an int, or a float
	switch samplerParamTag.VType {
	case model.Float64Type:
		return samplerParamTag.Float64(), nil
	case model.Int64Type:
		return float64(samplerParamTag.Int64()), nil
	default:
		return strconv.ParseFloat(samplerParamTag.AsString(), 64)
	}
}
