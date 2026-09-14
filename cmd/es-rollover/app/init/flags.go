// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"flag"
	"strings"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/featuregate"

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
	spanTotalFieldsLimit         = "span-total-fields-limit"
	readIndexPrefixes            = "index-prefixes-read"
)

// Config holds configuration for index cleaner binary.
// Config.IndexPrefix supersedes Indices.IndexPrefix
type Config struct {
	app.Config
	cfg.Indices

	// AdditionalReadPrefixes holds extra index prefixes (each already normalized
	// with a trailing "-") from --index-prefixes-read. For each one, a read alias
	// pointing at the rollover index created for Config.IndexPrefix is also
	// created, so readers using a different prefix convention can read the same
	// index. No write alias is created for these prefixes.
	AdditionalReadPrefixes []string
}

// AddFlags adds flags for TLS to the FlagSet.
func (*Config) AddFlags(flags *flag.FlagSet) {
	flags.Int(shards, 5, "Number of shards")
	flags.Int(replicas, 1, "Number of replicas")
	flags.Int(prioritySpanTemplate, 0, "Priority of jaeger-span index template (ESv8 only)")
	flags.Int(priorityServiceTemplate, 0, "Priority of jaeger-service index template (ESv8 only)")
	flags.Int(priorityDependenciesTemplate, 0, "Priority of jaeger-dependencies index template (ESv8 only)")
	flags.Int(prioritySamplingTemplate, 0, "Priority of jaeger-sampling index template (ESv8 only)")
	flags.Int64(spanTotalFieldsLimit, 0, "Sets index.mapping.total_fields.limit on the jaeger-span index template. If unset, no limit is set and Elasticsearch's own default applies")
	flags.String(readIndexPrefixes, "", "Comma-separated list of additional index prefixes. For each one, a read alias pointing at the rollover index created for --index-prefix is also created")
	// init installs the index templates, and a feature gate can change what they contain, so
	// it takes the same --feature-gates flag as the jaeger binary and esmapping-generator. The
	// flag writes straight into the global registry, so InitFromViper has nothing to read.
	featuregate.GlobalRegistry().RegisterFlags(flags)
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

	if v.IsSet(spanTotalFieldsLimit) {
		c.Indices.Spans.TotalFieldsLimit = new(v.GetInt64(spanTotalFieldsLimit))
	}

	// Config.IndexPrefix supersedes Indices.IndexPrefix: the client renders the
	// templates from Indices, so reconcile the prefix onto it here.
	c.Indices.IndexPrefix = cfg.IndexPrefix(c.Config.IndexPrefix)

	c.AdditionalReadPrefixes = nil
	for _, prefix := range strings.Split(v.GetString(readIndexPrefixes), ",") {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		c.AdditionalReadPrefixes = append(c.AdditionalReadPrefixes, prefix+"-")
	}
}
