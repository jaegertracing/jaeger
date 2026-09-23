// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/anonymizer"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/query"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/writer"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/version"
)

var logger, _ = zap.NewDevelopment()

func main() {
	options := app.Options{}

	command := &cobra.Command{
		Use:   "jaeger-anonymizer",
		Short: "Jaeger anonymizer hashes fields of a trace for easy sharing",
		Long:  `Jaeger anonymizer queries Jaeger query for a trace, anonymizes fields, and store in file`,
		Run: func(_ *cobra.Command, _ /* args */ []string) {
			if err := run(&options, logger); err != nil {
				logger.Fatal("anonymizer failed", zap.Error(err))
			}
		},
	}

	options.AddFlags(command)

	command.AddCommand(version.Command())

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// run captures one trace and writes it to three files under the output directory: the trace as
// captured, the trace anonymized, and the mapping from hashes back to the original names. Both
// traces are OTLP JSON, which Jaeger UI accepts as an upload.
func run(options *app.Options, logger *zap.Logger) error {
	// The trace ID is parsed once, so the ID queried is the ID in the files.
	v1TraceID, err := model.TraceIDFromString(options.TraceID)
	if err != nil {
		return fmt.Errorf("invalid trace ID %q: %w", options.TraceID, err)
	}
	traceID := v1adapter.FromV1TraceID(v1TraceID)

	prefix := options.OutputDir + "/" + options.TraceID
	capturedFile := prefix + ".original.json"
	anonymizedFile := prefix + ".anonymized.json"
	mappingFile := prefix + ".mapping.json"

	anon, err := anonymizer.New(mappingFile, anonymizer.Options{
		HashStandardTags: options.HashStandardTags,
		HashCustomTags:   options.HashCustomTags,
		HashLogs:         options.HashLogs,
		HashProcess:      options.HashProcess,
	}, logger)
	if err != nil {
		return err
	}

	q, err := query.New(options.QueryGRPCHostPort)
	if err != nil {
		return fmt.Errorf("error while creating query object: %w", err)
	}
	traces, err := q.QueryTrace(
		traceID,
		initTime(options.StartTime),
		initTime(options.EndTime),
		options.MaxSpansCount,
	)
	if closeErr := q.Close(); closeErr != nil {
		logger.Error("Failed to close grpc client connection", zap.Error(closeErr))
	}
	if err != nil {
		return fmt.Errorf("error while querying for trace: %w", err)
	}
	logger.Info("Captured trace", zap.Int("spans", traces.SpanCount()))

	if err := writer.WriteTraces(capturedFile, traces); err != nil {
		return err
	}
	logger.Sugar().Infof("Wrote captured trace to %s", capturedFile)

	anonymized := ptrace.NewTraces()
	traces.CopyTo(anonymized)
	anon.AnonymizeTraces(anonymized)
	if err := writer.WriteTraces(anonymizedFile, anonymized); err != nil {
		return err
	}
	logger.Sugar().Infof("Wrote anonymized trace to %s; it can be uploaded to Jaeger UI", anonymizedFile)

	return anon.SaveMapping()
}

func initTime(ts int64) time.Time {
	var t time.Time
	if ts != 0 {
		t = time.Unix(0, ts)
	}
	return t
}
