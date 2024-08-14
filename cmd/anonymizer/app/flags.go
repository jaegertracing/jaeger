// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
