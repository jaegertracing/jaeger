// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"time"

	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

var defaultIndexOptions = config.IndexOptions{
	Shards:   5,
	Replicas: new(int64(1)),
	Priority: 0,
	Rotation: config.RotationConfig{
		Periodic: configoptional.Default(config.PeriodicRotation{
			DateLayout:        initDateLayout("day", "-"),
			RolloverFrequency: "day",
		}),
	},
}

// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of Elasticsearch configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	Config config.Configuration `mapstructure:",squash"`
}

func initDateLayout(rolloverFreq, sep string) string {
	// default to daily format
	indexLayout := "2006" + sep + "01" + sep + "02"
	if rolloverFreq == "hour" {
		indexLayout = indexLayout + sep + "15"
	}
	return indexLayout
}

// defaultTagDotReplacement stands in for the dots of an attribute key stored as a field.
const defaultTagDotReplacement = "@"

func DefaultConfig() config.Configuration {
	return config.Configuration{
		Authentication: config.Authentication{},
		Sniffing: config.Sniffing{
			Enabled: false,
		},
		MaxSpanAge:               72 * time.Hour,
		MaxTraceDuration:         24 * time.Hour,
		AdaptiveSamplingLookback: 72 * time.Hour,
		BulkProcessing: config.BulkProcessing{
			MaxBytes:      5 * 1000 * 1000,
			Workers:       1,
			FlushInterval: time.Millisecond * 200,
		},
		// The deprecated top-level spelling decodes over the same dot replacement when a
		// configuration still uses it, so an explicitly empty one stays empty.
		Tags:               configoptional.Default(config.TagsAsFields{DotReplacement: defaultTagDotReplacement}),
		Enabled:            true,
		Version:            0,
		Servers:            []string{"http://127.0.0.1:9200"},
		RemoteReadClusters: []string{},
		MaxDocCount:        10_000,
		// Off by default: sorting on _id builds _id field data on the heap for every segment a
		// search touches, and the config field's comment says what that costs.
		SpanSearchTieBreakByID: false,
		LogLevel:               "error",
		CreateIndexTemplates:   true,
		HTTPCompression:        true,
		Indices: config.Indices{
			Spans: config.SpanIndexOptions{
				IndexOptions: defaultIndexOptions,
				// No attribute is stored as a field by default, but the dot replacement every
				// elevated key needs once one is selected is set.
				Tags: config.TagsAsFields{
					DotReplacement: defaultTagDotReplacement,
				},
			},
			Services:     defaultIndexOptions,
			Dependencies: defaultIndexOptions,
			Sampling:     defaultIndexOptions,
		},
	}
}
