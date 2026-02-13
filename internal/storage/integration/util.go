// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

func translateFixtureToOTLPTrace(inPath, outPath string) error {
	inStr, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var traceV1 model.Trace
	err = jsonpb.Unmarshal(bytes.NewReader(inStr), &traceV1)
	if err != nil {
		return err
	}
	trace := v1adapter.V1TraceToOtelTrace(&traceV1)
	validationTrace := modelTraceFromOtelTrace(trace)
	if err := validateTraces(&traceV1, validationTrace); err != nil {
		return err
	}
	marshaller := ptrace.JSONMarshaler{}
	outBytes, err := marshaller.MarshalTraces(trace)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = json.Indent(&buf, outBytes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o600)
}

// modelTraceFromOtelTrace extracts spans from otel traces
func modelTraceFromOtelTrace(otelTrace ptrace.Traces) *model.Trace {
	var spans []*model.Span
	batches := v1adapter.V1BatchesFromTraces(otelTrace)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				proc := *batch.Process // shallow clone
				span.Process = &proc
			}
			spans = append(spans, span)

			if span.Process.Tags == nil {
				span.Process.Tags = []model.KeyValue{}
			}

			if span.References == nil {
				span.References = []model.SpanRef{}
			}
			if span.Tags == nil {
				span.Tags = []model.KeyValue{}
			}
			if span.Logs == nil {
				span.Logs = []model.Log{}
			}
		}
	}
	return &model.Trace{Spans: spans}
}

func validateTraces(expected, actual *model.Trace) error {
	expectedBytes, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	actualBytes, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedBytes, actualBytes) {
		return fmt.Errorf("traces are not equal\n actual: %s\n expected: %s", string(actualBytes), string(expectedBytes))
	}
	return nil
}
