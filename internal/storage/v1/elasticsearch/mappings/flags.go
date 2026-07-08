// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"github.com/spf13/cobra"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

// Options represent configurable parameters for jaeger-esmapping-generator
type Options struct {
	Mapping       string
	EsVersion     uint
	Shards        int64
	Replicas      *int64
	IndexPrefix   string
	UseILM        string // using string as util is being used in python and using bool leads to type issues.
	ILMPolicyName string
}

// BackendVersion returns the BackendVersion corresponding to the EsVersion flag.
// TODO: This unsafe cast only works for Elasticsearch versions (6, 7, 8, 9).
// OpenSearch cannot be specified because its enum values (101, 102, 103) are
// internal. Add an --opensearch-version flag and use it to return the correct
// OpenSearch BackendVersion variant.
func (o *Options) BackendVersion() es.BackendVersion {
	return es.BackendVersion(o.EsVersion)
}

const (
	mappingFlag       = "mapping"
	esVersionFlag     = "es-version"
	shardsFlag        = "shards"
	replicasFlag      = "replicas"
	indexPrefixFlag   = "index-prefix"
	useILMFlag        = "use-ilm"
	ilmPolicyNameFlag = "ilm-policy-name"
)

// AddFlags adds flags for esmapping-generator main program
func (o *Options) AddFlags(command *cobra.Command) {
	command.Flags().StringVar(
		&o.Mapping,
		mappingFlag,
		"",
		"The index mapping the template will be applied to: one of jaeger-span, jaeger-service, jaeger-dependencies, or jaeger-sampling",
	)
	command.Flags().UintVar(
		&o.EsVersion,
		esVersionFlag,
		7,
		"The major Elasticsearch version",
	)
	command.Flags().Int64Var(
		&o.Shards,
		shardsFlag,
		5,
		"The number of shards per index in Elasticsearch",
	)
	// Allocate storage for Replicas so Int64Var can write into it.
	o.Replicas = new(int64)
	command.Flags().Int64Var(
		o.Replicas,
		replicasFlag,
		1,
		"The number of replicas per index in Elasticsearch",
	)
	command.Flags().StringVar(
		&o.IndexPrefix,
		indexPrefixFlag,
		"",
		"Specifies index prefix",
	)
	command.Flags().StringVar(
		&o.UseILM,
		useILMFlag,
		"false",
		"Set to true to use ILM for managing lifecycle of jaeger indices",
	)
	command.Flags().StringVar(
		&o.ILMPolicyName,
		ilmPolicyNameFlag,
		"jaeger-ilm-policy",
		"The name of the ILM policy to use if ILM is active",
	)

	// mark mapping flag as mandatory
	command.MarkFlagRequired(mappingFlag)
}
