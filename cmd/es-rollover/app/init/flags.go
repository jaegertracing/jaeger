// Copyright (c) 2021 The Jaeger Authors.
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

package init

import (
	"flag"

	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
)

const (
	spanShards                   = "shards-span"
	serviceShards                = "shards-service"
	dependenciesShards           = "shards-dependencies"
	samplingShards               = "shards-sampling"
	spanReplicas                 = "replicas-span"
	serviceReplicas              = "replicas-service"
	dependenciesReplicas         = "replicas-dependencies"
	samplingReplicas             = "replicas-sampling"
	prioritySpanTemplate         = "priority-span-template"
	priorityServiceTemplate      = "priority-service-template"
	priorityDependenciesTemplate = "priority-dependencies-template"
	prioritySamplingTemplate     = "priority-sampling-template"
)

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	cfg.Indices
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.Int64(spanShards, 5, "Number of span index shards")
	flags.Int64(serviceShards, 5, "Number of service index shards")
	flags.Int64(dependenciesShards, 5, "Number of dependencies index shards")
	flags.Int64(samplingShards, 5, "Number of sampling index shards")

	flags.Int64(spanReplicas, 1, "Number of span index replicas")
	flags.Int64(serviceReplicas, 1, "Number of services index replicas")
	flags.Int64(dependenciesReplicas, 1, "Number of dependencies index replicas")
	flags.Int64(samplingReplicas, 1, "Number of sampling index replicas")

	flags.Int64(prioritySpanTemplate, 0, "Priority of jaeger-span index template (ESv8 only)")
	flags.Int64(priorityServiceTemplate, 0, "Priority of jaeger-service index template (ESv8 only)")
	flags.Int64(priorityDependenciesTemplate, 0, "Priority of jaeger-dependencies index template (ESv8 only)")
	flags.Int64(prioritySamplingTemplate, 0, "Priority of jaeger-sampling index template (ESv8 only)")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.Indices.Spans.TemplateOptions.NumShards = v.GetInt(spanShards)
	c.Indices.Spans.TemplateOptions.NumReplicas = v.GetInt(spanReplicas)
	c.Indices.Services.TemplateOptions.NumShards = v.GetInt(serviceShards)
	c.Indices.Services.TemplateOptions.NumReplicas = v.GetInt(serviceReplicas)
	c.Indices.Dependencies.TemplateOptions.NumShards = v.GetInt(dependenciesShards)
	c.Indices.Dependencies.TemplateOptions.NumReplicas = v.GetInt(dependenciesReplicas)
	c.Indices.Sampling.TemplateOptions.NumShards = v.GetInt(samplingShards)
	c.Indices.Sampling.TemplateOptions.NumReplicas = v.GetInt(samplingReplicas)
	c.Indices.Spans.TemplateOptions.Priority = v.GetInt(prioritySpanTemplate)
	c.Indices.Services.TemplateOptions.Priority = v.GetInt(priorityServiceTemplate)
	c.Indices.Dependencies.TemplateOptions.Priority = v.GetInt(priorityDependenciesTemplate)
	c.Indices.Sampling.TemplateOptions.Priority = v.GetInt(prioritySamplingTemplate)
}
