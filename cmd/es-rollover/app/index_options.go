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
	Prefix       string
	TemplateName string
}

// RolloverIndices return an array of indices to rollover
func RolloverIndices(archive bool, prefix string) []IndexOption {
	if archive {
		return []IndexOption{
			{
				Prefix:       strings.TrimLeft(fmt.Sprintf("%s-jaeger-span-archive", prefix), "-"),
				TemplateName: "jaeger-span",
			},
		}
	}
	return []IndexOption{
		{
			Prefix:       strings.TrimLeft(fmt.Sprintf("%s-jaeger-span", prefix), "-"),
			TemplateName: "jaeger-span",
		},
		{
			Prefix:       strings.TrimLeft(fmt.Sprintf("%s-jaeger-service", prefix), "-"),
			TemplateName: "jaeger-service",
		},
	}
}

// ReadAliasName returns read alias name of the index
func (i *IndexOption) ReadAliasName() string {
	return fmt.Sprintf(readAliasFormat, i.Prefix)
}

// WriteAliasName returns write alias name of the index
func (i *IndexOption) WriteAliasName() string {
	return fmt.Sprintf(writeAliasFormat, i.Prefix)
}

// InitialRolloverIndex returns the initial index rollover name
func (i *IndexOption) InitialRolloverIndex() string {
	return fmt.Sprintf(rolloverIndexFormat, i.Prefix)
}
