// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	model2otel "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
)

func modelToOTLP(spans []*model.Span) (ptrace.Traces, error) {
	batch := &model.Batch{Spans: spans}
	return model2otel.ProtoToTraces([]*model.Batch{batch})
}
