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
	Version       es.BackendVersion
	Shards        int64
	Replicas      *int64
	IndexPrefix   string
	UseILM        string // using string as util is being used in python and using bool leads to type issues.
	ILMPolicyName string
}

// resolveBackendVersion selects the backend version from the generator's two
// version flags: the distribution-aware --version token ("es8", "os3") wins when
// set, otherwise the legacy numeric --es-version flag (an Elasticsearch major
// version; OpenSearch is only reachable through --version) is used.
func resolveBackendVersion(versionToken string, legacyEsVersion uint) (es.BackendVersion, error) {
	if versionToken != "" {
		return es.ParseBackendVersion(versionToken)
	}
	return es.BackendVersion(legacyEsVersion), nil
}

const (
	mappingFlag       = "mapping"
	versionFlag       = "version"
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
	// The two version flags are transient parsing inputs rather than Options
	// fields: the PreRunE hook below resolves them into the single typed Version.
	var versionToken string
	var legacyEsVersion uint
	command.Flags().StringVar(
		&versionToken,
		versionFlag,
		"",
		"The backend distribution and major version to render for: one of es7, es8, es9, os1, os2, os3 (es=Elasticsearch, os=OpenSearch). Takes precedence over --es-version.",
	)
	command.Flags().UintVar(
		&legacyEsVersion,
		esVersionFlag,
		7,
		"The major Elasticsearch version (legacy; prefer --version, which can also select OpenSearch)",
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

	// Resolve the transient version flags into the typed Version field before the
	// command runs, surfacing an invalid --version as a command error.
	command.PreRunE = func(_ *cobra.Command, _ []string) error {
		version, err := resolveBackendVersion(versionToken, legacyEsVersion)
		if err != nil {
			return err
		}
		o.Version = version
		return nil
	}
}
