// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"fmt"

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
// version flags. The distribution-aware --backend token ("es8", "os3") wins when
// set; otherwise the legacy numeric --es-version is used. The legacy flag still
// accepts OpenSearch codes (101-103) for backward compatibility — --backend is
// just the readable spelling — so the number is validated against the supported
// set rather than cast blindly (an unsupported value like 999 would otherwise be
// misread as OpenSearch, since IsOpenSearch is >= 101).
func resolveBackendVersion(backendToken string, legacyEsVersion uint) (es.BackendVersion, error) {
	if backendToken != "" {
		return es.ParseBackendVersion(backendToken)
	}
	if !es.IsSupportedVersion(legacyEsVersion) {
		return 0, fmt.Errorf("unsupported --es-version %d: expected 7, 8, 9 (Elasticsearch) or 101, 102, 103 (OpenSearch); prefer --backend", legacyEsVersion)
	}
	return es.BackendVersion(legacyEsVersion), nil
}

const (
	mappingFlag       = "mapping"
	backendFlag       = "backend"
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
	var backendToken string
	var legacyEsVersion uint
	command.Flags().StringVar(
		&backendToken,
		backendFlag,
		"",
		"The backend distribution and major version to render for: one of es7, es8, es9, os1, os2, os3 (es=Elasticsearch, os=OpenSearch). Takes precedence over --es-version.",
	)
	command.Flags().UintVar(
		&legacyEsVersion,
		esVersionFlag,
		7,
		"The backend version as a numeric code: 7, 8, 9 (Elasticsearch) or 101, 102, 103 (OpenSearch). Legacy; prefer the more readable --backend.",
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
	// command runs, surfacing an invalid --backend as a command error. The warning
	// goes to stderr so it never corrupts the rendered template on stdout.
	command.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd.Flags().Changed(esVersionFlag) && !cmd.Flags().Changed(backendFlag) {
			cmd.PrintErrln("Warning: --es-version is deprecated; use --backend instead (e.g. --backend es8 or --backend os3).")
		}
		version, err := resolveBackendVersion(backendToken, legacyEsVersion)
		if err != nil {
			return err
		}
		o.Version = version
		return nil
	}
}
