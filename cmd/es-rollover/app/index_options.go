// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"strings"

	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
)

const (
	writeAliasFormat    = "%s-write"
	readAliasFormat     = "%s-read"
	rolloverIndexFormat = "%s-000001"
)

// IndexOption holds the information for the indices to rollover
type IndexOption struct {
	prefix    string
	indexType string
	Mapping   string
}

// RolloverIndices return an array of indices to rollover
func RolloverIndices(archive bool, skipDependencies bool, adaptiveSampling bool, indices cfg.Indices) []IndexOption {
	if archive {
		return []IndexOption{
			{
				prefix:    indices.Spans.Prefix,
				indexType: "jaeger-span-archive",
				Mapping:   "jaeger-span",
			},
		}
	}

	indexOptions := []IndexOption{
		{
			prefix:    indices.Spans.Prefix,
			Mapping:   "jaeger-span",
			indexType: "jaeger-span",
		},
		{
			prefix:    indices.Services.Prefix,
			Mapping:   "jaeger-service",
			indexType: "jaeger-service",
		},
	}

	if !skipDependencies {
		indexOptions = append(indexOptions, IndexOption{
			prefix:    indices.Dependencies.Prefix,
			Mapping:   "jaeger-dependencies",
			indexType: "jaeger-dependencies",
		})
	}

	if adaptiveSampling {
		indexOptions = append(indexOptions, IndexOption{
			prefix:    indices.Sampling.Prefix,
			Mapping:   "jaeger-sampling",
			indexType: "jaeger-sampling",
		})
	}

	return indexOptions
}

func (i *IndexOption) IndexName() string {
	return strings.TrimLeft(fmt.Sprintf("%s%s", i.prefix, i.indexType), "-")
}

// ReadAliasName returns read alias name of the index
func (i *IndexOption) ReadAliasName() string {
	return fmt.Sprintf(readAliasFormat, i.IndexName())
}

// WriteAliasName returns write alias name of the index
func (i *IndexOption) WriteAliasName() string {
	return fmt.Sprintf(writeAliasFormat, i.IndexName())
}

// InitialRolloverIndex returns the initial index rollover name
func (i *IndexOption) InitialRolloverIndex() string {
	return fmt.Sprintf(rolloverIndexFormat, i.IndexName())
}

// TemplateName returns the prefixed template name
func (i *IndexOption) TemplateName() string {
	return strings.TrimLeft(fmt.Sprintf("%s%s", i.prefix, i.Mapping), "-")
}
