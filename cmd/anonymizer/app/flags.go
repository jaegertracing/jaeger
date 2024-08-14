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

package app

import (
	"github.com/spf13/cobra"
)

// Options represent configurable parameters for jaeger-anonymizer
type Options struct {
	QueryGRPCHostPort string
	MaxSpansCount     int
	TraceID           string
	OutputDir         string
	HashStandardTags  bool
	HashCustomTags    bool
	HashLogs          bool
	HashProcess       bool
}

const (
	queryGRPCHostPortFlag = "query-host-port"
	outputDirFlag         = "output-dir"
	traceIDFlag           = "trace-id"
	hashStandardTagsFlag  = "hash-standard-tags"
	hashCustomTagsFlag    = "hash-custom-tags"
	hashLogsFlag          = "hash-logs"
	hashProcessFlag       = "hash-process"
	maxSpansCount         = "max-spans-count"
)

// AddFlags adds flags for anonymizer main program
func (o *Options) AddFlags(command *cobra.Command) {
	command.Flags().StringVar(
		&o.QueryGRPCHostPort,
		queryGRPCHostPortFlag,
		"localhost:16686",
		"The host:port of the jaeger-query endpoint")
	command.Flags().StringVar(
		&o.OutputDir,
		outputDirFlag,
		"/tmp",
		"The directory to store the anonymized trace")
	command.Flags().StringVar(
		&o.TraceID,
		traceIDFlag,
		"",
		"The trace-id of trace to anonymize")
	command.Flags().BoolVar(
		&o.HashStandardTags,
		hashStandardTagsFlag,
		false,
		"Whether to hash standard tags")
	command.Flags().BoolVar(
		&o.HashCustomTags,
		hashCustomTagsFlag,
		false,
		"Whether to hash custom tags")
	command.Flags().BoolVar(
		&o.HashLogs,
		hashLogsFlag,
		false,
		"Whether to hash logs")
	command.Flags().BoolVar(
		&o.HashProcess,
		hashProcessFlag,
		false,
		"Whether to hash process")
	command.Flags().IntVar(
		&o.MaxSpansCount,
		maxSpansCount,
		-1,
		"The maximum number of spans to anonymize")

	// mark traceid flag as mandatory
	command.MarkFlagRequired(traceIDFlag)
}
