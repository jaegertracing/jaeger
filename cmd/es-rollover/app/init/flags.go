// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

const (
	shards                       = "shards"
	replicas                     = "replicas"
	prioritySpanTemplate         = "priority-span-template"
	priorityServiceTemplate      = "priority-service-template"
	priorityDependenciesTemplate = "priority-dependencies-template"
	prioritySamplingTemplate     = "priority-sampling-template"
)

// Config holds configuration for index cleaner binary.
// Config.IndexPrefix supersedes Indices.IndexPrefix
type Config struct {
	app.Config
	cfg.Indices
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.Int(shards, 5, "Number of shards")
	flags.Int(replicas, 1, "Number of replicas")
	flags.Int(prioritySpanTemplate, 0, "Priority of jaeger-span index template (ESv8 only)")
	flags.Int(priorityServiceTemplate, 0, "Priority of jaeger-service index template (ESv8 only)")
	flags.Int(priorityDependenciesTemplate, 0, "Priority of jaeger-dependencies index template (ESv8 only)")
	flags.Int(prioritySamplingTemplate, 0, "Priority of jaeger-sampling index template (ESv8 only)")
}

// InitFromViper initializes config from viper.Viper.
func (c *Config) InitFromViper(v *viper.Viper) {
	c.Indices.Spans.Shards = v.GetInt64(shards)
	c.Indices.Services.Shards = v.GetInt64(shards)
	c.Indices.Dependencies.Shards = v.GetInt64(shards)
	c.Indices.Sampling.Shards = v.GetInt64(shards)

	repsPtr := new(v.GetInt64(replicas))
	c.Indices.Spans.Replicas = repsPtr
	c.Indices.Services.Replicas = repsPtr
	c.Indices.Dependencies.Replicas = repsPtr
	c.Indices.Sampling.Replicas = repsPtr

	c.Indices.Spans.Priority = v.GetInt64(prioritySpanTemplate)
	c.Indices.Services.Priority = v.GetInt64(priorityServiceTemplate)
	c.Indices.Dependencies.Priority = v.GetInt64(priorityDependenciesTemplate)
	c.Indices.Sampling.Priority = v.GetInt64(prioritySamplingTemplate)
}
