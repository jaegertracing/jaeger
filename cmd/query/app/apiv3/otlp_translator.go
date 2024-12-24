// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
)

func modelToOTLP(spans []*model.Span) ptrace.Traces {
	batch := &model.Batch{Spans: spans}
	tr := jptrace.ProtoToTraces([]*model.Batch{batch})
	return tr
}
