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

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

// Options represent configurable parameters for jaeger-esmapping-generator
type Options struct {
	Mapping       string
	EsVersion     uint
	Indices       config.Indices
	IndexPrefix   string
	UseILM        string // using string as util is being used in python and using bool leads to type issues.
	ILMPolicyName string
}

const (
	mappingFlag       = "mapping"
	esVersionFlag     = "es-version"
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
		"The index mapping the template will be applied to. Pass either jaeger-span or jaeger-service")
	command.Flags().UintVar(
		&o.EsVersion,
		esVersionFlag,
		7,
		"The major Elasticsearch version")
	command.Flags().IntVar(
		&o.Indices.Spans.TemplateNumShards,
		config.GetNumShardSpanFlag(),
		5,
		"The number of shards per span index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Spans.TemplateNumReplicas,
		config.GetNumReplicasSpanFlag(),
		1,
		"The number of replicas per index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Services.TemplateNumShards,
		config.GetNumShardServiceFlag(),
		5,
		"The number of shards per service index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Services.TemplateNumReplicas,
		config.GetNumReplicasServiceFlag(),
		1,
		"The number of replicas per service index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Dependencies.TemplateNumShards,
		config.GetNumShardDependenciesFlag(),
		5,
		"The number of shards per dependencies index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Dependencies.TemplateNumReplicas,
		config.GetNumReplicasDependenciesFlag(),
		1,
		"The number of replicas per dependencies index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Sampling.TemplateNumShards,
		config.GetNumShardSamplingFlag(),
		5,
		"The number of shards per sampling index in Elasticsearch")
	command.Flags().IntVar(
		&o.Indices.Sampling.TemplateNumReplicas,
		config.GetNumReplicasSamplingFlag(),
		1,
		"The number of replicas per sampling index in Elasticsearch")
	command.Flags().StringVar(
		&o.IndexPrefix,
		indexPrefixFlag,
		"",
		"Specifies index prefix")
	command.Flags().StringVar(
		&o.UseILM,
		useILMFlag,
		"false",
		"Set to true to use ILM for managing lifecycle of jaeger indices")
	command.Flags().StringVar(
		&o.ILMPolicyName,
		ilmPolicyNameFlag,
		"jaeger-ilm-policy",
		"The name of the ILM policy to use if ILM is active")

	// mark mapping flag as mandatory
	command.MarkFlagRequired(mappingFlag)
}
