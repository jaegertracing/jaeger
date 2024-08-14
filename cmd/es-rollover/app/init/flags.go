// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
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
type Config struct {
	app.Config
	Shards                       int
	Replicas                     int
	PrioritySpanTemplate         int
	PriorityServiceTemplate      int
	PriorityDependenciesTemplate int
	PrioritySamplingTemplate     int
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
	c.Shards = v.GetInt(shards)
	c.Replicas = v.GetInt(replicas)
	c.PrioritySpanTemplate = v.GetInt(prioritySpanTemplate)
	c.PriorityServiceTemplate = v.GetInt(priorityServiceTemplate)
	c.PriorityDependenciesTemplate = v.GetInt(priorityDependenciesTemplate)
	c.PrioritySamplingTemplate = v.GetInt(prioritySamplingTemplate)
}
