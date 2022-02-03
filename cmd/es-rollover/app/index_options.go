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

package app

import (
	"fmt"
	"strings"
)

const writeAliasFormat = "%s-write"
const readAliasFormat = "%s-read"
const rolloverIndexFormat = "%s-000001"

// IndexOption holds the information for the indices to rollover
type IndexOption struct {
	prefix    string
	indexType string
	Mapping   string
}

// RolloverIndices return an array of indices to rollover
func RolloverIndices(archive bool, prefix string) []IndexOption {
	if archive {
		return []IndexOption{
			{
				prefix:    prefix,
				indexType: "jaeger-span-archive",
				Mapping:   "jaeger-span",
			},
		}
	}
	return []IndexOption{
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
		{
			prefix:    prefix,
			Mapping:   "jaeger-dependencies",
			indexType: "jaeger-dependencies",
		},
	}
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
