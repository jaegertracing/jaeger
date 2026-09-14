// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"strings"
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
func RolloverIndices(archive bool, skipDependencies bool, adaptiveSampling bool, prefix string) []IndexOption {
	if archive {
		return []IndexOption{
			{
				prefix:    prefix,
				indexType: "jaeger-span-archive",
				Mapping:   "jaeger-span",
			},
		}
	}

	indexOptions := []IndexOption{
		{
			prefix:    prefix,
			Mapping:   "jaeger-span",
			indexType: "jaeger-span",
		},
		{
			prefix:    prefix,
			Mapping:   "jaeger-service",
			indexType: "jaeger-service",
		},
	}

	if !skipDependencies {
		indexOptions = append(indexOptions, IndexOption{
			prefix:    prefix,
			Mapping:   "jaeger-dependencies",
			indexType: "jaeger-dependencies",
		})
	}

	if adaptiveSampling {
		indexOptions = append(indexOptions, IndexOption{
			prefix:    prefix,
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

// ReadAliasNameWithPrefix returns the read alias name for this index's type,
// substituting prefix for the option's own prefix. It lets an additional index
// prefix (see --index-prefixes-read) be attached as a read alias pointing at
// the same underlying rollover index created for the option's own prefix.
func (i *IndexOption) ReadAliasNameWithPrefix(prefix string) string {
	indexName := strings.TrimLeft(fmt.Sprintf("%s%s", prefix, i.indexType), "-")
	return fmt.Sprintf(readAliasFormat, indexName)
}

// InitialRolloverIndex returns the initial index rollover name
func (i *IndexOption) InitialRolloverIndex() string {
	return fmt.Sprintf(rolloverIndexFormat, i.IndexName())
}

// TemplateName returns the prefixed template name
func (i *IndexOption) TemplateName() string {
	return strings.TrimLeft(fmt.Sprintf("%s%s", i.prefix, i.Mapping), "-")
}
