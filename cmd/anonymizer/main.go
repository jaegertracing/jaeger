// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/anonymizer/app"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/anonymizer"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/query"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/writer"
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
			prefix := options.OutputDir + "/" + options.TraceID
			conf := writer.Config{
				MaxSpansCount:  options.MaxSpansCount,
				CapturedFile:   prefix + ".original.json",
				AnonymizedFile: prefix + ".anonymized.json",
				MappingFile:    prefix + ".mapping.json",
				AnonymizerOpts: anonymizer.Options{
					HashStandardTags: options.HashStandardTags,
					HashCustomTags:   options.HashCustomTags,
					HashLogs:         options.HashLogs,
					HashProcess:      options.HashProcess,
				},
			}

			w, err := writer.New(conf, logger)
			if err != nil {
				logger.Fatal("error while creating writer object", zap.Error(err))
			}

			query, err := query.New(options.QueryGRPCHostPort)
			if err != nil {
				logger.Fatal("error while creating query object", zap.Error(err))
			}

			traces, err := query.QueryTrace(
				options.TraceID,
				initTime(options.StartTime),
				initTime(options.EndTime),
			)
			if err != nil {
				logger.Fatal("error while querying for trace", zap.Error(err))
			}
			if err := query.Close(); err != nil {
				logger.Error("Failed to close grpc client connection", zap.Error(err))
			}

			if err := w.WriteTraces(traces); err != nil {
				if errors.Is(err, writer.ErrMaxSpansCountReached) {
					logger.Info("max spans count reached")
				} else {
					logger.Error("error while writing traces", zap.Error(err))
				}
			}
			w.Close()
			logger.Sugar().Infof("Wrote anonymized trace to %s; it can be uploaded to Jaeger UI", conf.AnonymizedFile)
		},
	}

	options.AddFlags(command)

	command.AddCommand(version.Command())

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func initTime(ts int64) time.Time {
	var t time.Time
	if ts != 0 {
		t = time.Unix(0, ts)
	}
	return t
}
