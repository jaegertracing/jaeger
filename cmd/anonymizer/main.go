// Copyright (c) 2020 The Jaeger Authors.
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

package main

import (
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
	var options = app.Options{}

	var command = &cobra.Command{
		Use:   "jaeger-anonymizer",
		Short: "Jaeger anonymizer hashes fields of a trace for easy sharing",
		Long:  `Jaeger anonymizer queries Jaeger query for a trace, anonymizes fields, and store in file`,
		Run: func(cmd *cobra.Command, args []string) {
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

			writer, err := writer.New(conf, logger)
			if err != nil {
				logger.Fatal("error while creating writer object", zap.Error(err))
			}

			query, err := query.New(options.QueryGRPCHostPort)
			if err != nil {
				logger.Fatal("error while creating query object", zap.Error(err))
			}

			spans, err := query.QueryTrace(options.TraceID)
			if err != nil {
				logger.Fatal("error while querying for trace", zap.Error(err))
			}

			for _, span := range spans {
				writer.WriteSpan(&span)
			}
			writer.Close()

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

	if error := command.Execute(); error != nil {
		fmt.Println(error.Error())
		os.Exit(1)
	}
}
