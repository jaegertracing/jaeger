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

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
)

// Config holds configuration for index cleaner binary.
type Config struct {
	app.Config
	cfg.Indices
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.Int64(cfg.GetNumShardSpanFlag(), 5, "Number of span index shards")
	flags.Int64(cfg.GetNumShardServiceFlag(), 5, "Number of service index shards")
	flags.Int64(cfg.GetNumShardDependenciesFlag(), 5, "Number of dependencies index shards")
	flags.Int64(cfg.GetNumShardSamplingFlag(), 5, "Number of sampling index shards")

	flags.Int64(cfg.GetNumReplicasSpanFlag(), 1, "Number of span index replicas")
	flags.Int64(cfg.GetNumReplicasServiceFlag(), 1, "Number of services index replicas")
	flags.Int64(cfg.GetNumReplicasDependenciesFlag(), 1, "Number of dependencies index replicas")
	flags.Int64(cfg.GetNumReplicasSamplingFlag(), 1, "Number of sampling index replicas")

	flags.Int64(cfg.GetPrioritySpanTemplate(), 0, "Priority of jaeger-span index template (ESv8 only)")
	flags.Int64(cfg.GetPriorityServiceTemplate(), 0, "Priority of jaeger-service index template (ESv8 only)")
	flags.Int64(cfg.GetPriorityDependenciesTemplate(), 0, "Priority of jaeger-dependencies index template (ESv8 only)")
	flags.Int64(cfg.GetPrioritySamplingTemplate(), 0, "Priority of jaeger-sampling index template (ESv8 only)")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.Indices.Spans.TemplateNumShards = v.GetInt(cfg.GetNumShardSpanFlag())
	c.Indices.Spans.TemplateNumReplicas = v.GetInt(cfg.GetNumReplicasSpanFlag())
	c.Indices.Services.TemplateNumShards = v.GetInt(cfg.GetNumShardServiceFlag())
	c.Indices.Services.TemplateNumReplicas = v.GetInt(cfg.GetNumReplicasServiceFlag())
	c.Indices.Dependencies.TemplateNumShards = v.GetInt(cfg.GetNumShardDependenciesFlag())
	c.Indices.Dependencies.TemplateNumReplicas = v.GetInt(cfg.GetNumReplicasDependenciesFlag())
	c.Indices.Sampling.TemplateNumShards = v.GetInt(cfg.GetNumShardSamplingFlag())
	c.Indices.Sampling.TemplateNumReplicas = v.GetInt(cfg.GetNumShardSamplingFlag())
	c.Indices.Spans.TemplatePriority = v.GetInt(cfg.GetPrioritySpanTemplate())
	c.Indices.Services.TemplatePriority = v.GetInt(cfg.GetPriorityServiceTemplate())
	c.Indices.Dependencies.TemplatePriority = v.GetInt(cfg.GetPriorityDependenciesTemplate())
	c.Indices.Sampling.TemplatePriority = v.GetInt(cfg.GetPrioritySamplingTemplate())
}
