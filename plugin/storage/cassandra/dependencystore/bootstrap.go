// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
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

package dependencystore

import (
	"github.com/jaegertracing/jaeger/pkg/cassandra"
)

// GetDependencyVersion attempts to determine the version of the dependencies table.
// TODO: Remove this once we've migrated to V2 permanently. https://github.com/jaegertracing/jaeger/issues/1344
func GetDependencyVersion(s cassandra.Session) Version {
	if err := s.Query("SELECT ts from dependencies_v2 limit 1;").Exec(); err != nil {
		return V1
	}
	return V2
}
