// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
)

var _ spanstore.Writer = (*spanWriter)(nil)

// SpanWriter utilizes the OTLP exporter to send span data to the Jaeger-v2 receiver
type spanWriter struct {
	testbed.TraceDataSender
}

func createSpanWriter() (*spanWriter, error) {
	sender := testbed.NewOTLPTraceDataSender(testbed.DefaultHost, testbed.DefaultOTLPPort)
	err := sender.Start()
	if err != nil {
		return nil, err
	}

	return &spanWriter{
		TraceDataSender: sender,
	}, nil
}

func (w *spanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	td, err := jaeger2otlp.ProtoToTraces([]*model.Batch{
		{
			Spans:   []*model.Span{span},
			Process: span.Process,
		},
	})
	if err != nil {
		return err
	}

	return w.ConsumeTraces(ctx, td)
}
