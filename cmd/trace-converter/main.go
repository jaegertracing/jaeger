// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

const (
	inFile     = "in-file"
	outFile    = "out-file"
	rootPath   = "root-path"
	fileFormat = "%s.json"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "jaeger-trace-converter",
		Short: "Jaeger trace-converter converts v1 trace fixtures to OTLP trace fixtures",
		Long:  "Jaeger trace-converter converts v1 trace fixtures to OTLP trace fixtures",
	}
	flags := rootCmd.Flags()
	flags.String(inFile, "", "The name of file to read from (where v1 fixtures are located without .json)")
	flags.String(outFile, "", "The name of file where otlp fixtures are needed to be written (without .json). If given empty, fixtures will be written to input file")
	flags.String(rootPath, "", "The root path to use to convert the traces to OTLP fixtures")
	if err := rootCmd.MarkFlagRequired(inFile); err != nil {
		panic(err)
	}
	if err := rootCmd.MarkFlagRequired(rootPath); err != nil {
		panic(err)
	}
	rootCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		in, err := cmd.Flags().GetString(inFile)
		if err != nil {
			return err
		}
		out, err := cmd.Flags().GetString(outFile)
		if err != nil {
			return err
		}
		root, err := cmd.Flags().GetString(rootPath)
		if err != nil {
			return err
		}
		if out == "" {
			out = in
		}
		return translateFixtureToOTLPTrace(root, in, out)
	}
	return rootCmd
}

func translateFixtureToOTLPTrace(root, in, out string) error {
	inPath := filepath.Join(root, fmt.Sprintf(fileFormat, in))
	outPath := filepath.Join(root, fmt.Sprintf(fileFormat, out))
	inStr, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var traceV1 model.Trace
	if err := jsonpb.Unmarshal(bytes.NewReader(inStr), &traceV1); err != nil {
		return err
	}
	trace := v1adapter.V1TraceToOtelTrace(&traceV1)
	expected := modelTraceFromOtelTrace(trace)
	if err := validateTraces(expected, &traceV1); err != nil {
		return err
	}
	marshaller := ptrace.JSONMarshaler{}
	outBytes, err := marshaller.MarshalTraces(trace)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, outBytes, "", "  "); err != nil {
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
	model.SortTrace(expected)
	model.SortTrace(actual)
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
