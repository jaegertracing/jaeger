package app

import (
	"github.com/spf13/cobra"
)

var QueryGRPCPort, MaxSpansCount int
var QueryGRPCHost, TraceID, OutputDir string
var HashStandardTags, HashCustomTags, HashLogs, HashProcess bool

const (
	queryGRPCHostFlag    = "query.host"
	queryGRPCPortFlag    = "query.port"
	outputDirFlag        = "output.dir"
	traceIDFlag          = "trace.id"
	hashStandardTagsFlag = "hash.standardtags"
	hashCustomTagsFlag   = "hash.customtags"
	hashLogsFlag         = "hash.logs"
	hashProcessFlag      = "hash.process"
	maxSpansCount        = "max.spanscount"
)

// AddFlags adds flags for anonymizer main program
func AddFlags(command *cobra.Command) {
	command.Flags().StringVar(
		&QueryGRPCHost,
		queryGRPCHostFlag,
		DefaultQueryGRPCHost,
		"hostname of the jaeger-query endpoint")
	command.Flags().IntVar(
		&QueryGRPCPort,
		queryGRPCPortFlag,
		DefaultQueryGRPCPort,
		"port of the jaeger-query endpoint")
	command.Flags().StringVar(
		&OutputDir,
		outputDirFlag,
		DefaultOutputDir,
		"directory to store the anonymized trace")
	command.Flags().StringVar(
		&TraceID,
		traceIDFlag,
		"",
		"trace-id of trace to anonymize")
	command.Flags().BoolVar(
		&HashStandardTags,
		hashStandardTagsFlag,
		DefaultHashStandardTags,
		"whether to hash standard tags")
	command.Flags().BoolVar(
		&HashCustomTags,
		hashCustomTagsFlag,
		DefaultHashCustomTags,
		"whether to hash custom tags")
	command.Flags().BoolVar(
		&HashLogs,
		hashLogsFlag,
		DefaultHashLogs,
		"whether to hash logs")
	command.Flags().BoolVar(
		&HashProcess,
		hashProcessFlag,
		DefaultHashProcess,
		"whether to hash process")
	command.Flags().IntVar(
		&MaxSpansCount,
		maxSpansCount,
		DefaultMaxSpansCount,
		"maximum number of spans to anonymize")

	// mark traceid flag as mandatory
	command.MarkFlagRequired(traceIDFlag)
}