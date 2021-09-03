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

type IndexOptions struct {
	Prefix       string
	TemplateName string
}

func RolloverIndices(archive bool, prefix string) []IndexOptions {
	if archive {
		return []IndexOptions{
			{
				Prefix:       strings.TrimLeft(fmt.Sprintf("%s-jaeger-span-archive", prefix), "-"),
				TemplateName: "jaeger-span",
			},
		}
	} else {
		return []IndexOptions{
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
}

func (i *IndexOptions) ReadAliasName() string {
	return fmt.Sprintf(readAliasFormat, i.Prefix)
}

func (i *IndexOptions) WriteAliasName() string {
	return fmt.Sprintf(writeAliasFormat, i.Prefix)
}

func (i *IndexOptions) InitialRolloverIndex() string {
	return fmt.Sprintf(rolloverIndexFormat, i.Prefix)
}
