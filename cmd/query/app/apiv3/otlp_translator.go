// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

func modelToOTLP(spans []*model.Span) ptrace.Traces {
	batch := &model.Batch{Spans: spans}
	tr := v1adapter.ProtoToTraces([]*model.Batch{batch})
	return tr
}
