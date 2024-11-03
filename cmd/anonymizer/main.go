// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/anonymizer/app"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/anonymizer"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/query"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/uiconv"
	"github.com/jaegertracing/jaeger/cmd/anonymizer/app/writer"
	"github.com/jaegertracing/jaeger/pkg/version"
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

			spans, err := query.QueryTrace(options.TraceID, options.StartTime, options.EndTime)
			if err != nil {
				logger.Fatal("error while querying for trace", zap.Error(err))
			}
			if err := query.Close(); err != nil {
				logger.Error("Failed to close grpc client connection", zap.Error(err))
			}

			for _, span := range spans {
				if err := w.WriteSpan(&span); err != nil {
					if errors.Is(err, writer.ErrMaxSpansCountReached) {
						logger.Info("max spans count reached")
						os.Exit(0)
					}
					logger.Error("error while writing span", zap.Error(err))
				}
			}
			w.Close()

			uiCfg := uiconv.Config{
				CapturedFile: conf.AnonymizedFile,
				UIFile:       prefix + ".anonymized-ui-trace.json",
				TraceID:      options.TraceID,
			}
			if err := uiconv.Extract(uiCfg, logger); err != nil {
				logger.Fatal("error while extracing UI trace", zap.Error(err))
			}
			logger.Sugar().Infof("Wrote UI-compatible anonymized file to %s", uiCfg.UIFile)
		},
	}

	options.AddFlags(command)

	command.AddCommand(version.Command())

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
