// Copyright (c) 2020 The Jaeger Authors.
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

package config

import "fmt"

type IndexType string

const (
	SPAN         IndexType = "span"
	SERVICE      IndexType = "service"
	DEPENDENCIES IndexType = "dependencies"
	SAMPLING     IndexType = "sampling"

	numShards   = "num-shards-"
	numReplicas = "num-replicas-"
	priority    = "priority-%s-template"
)

func NumShardSpanFlag() string {
	return getNumShardPrefix(SPAN)
}

func GetNumShardServiceFlag() string {
	return getNumShardPrefix(SERVICE)
}

func GetNumShardSamplingFlag() string {
	return getNumShardPrefix(SAMPLING)
}

func GetNumShardDependenciesFlag() string {
	return getNumShardPrefix(DEPENDENCIES)
}

func GetNumReplicasSpanFlag() string {
	return getNumReplicasPrefix(SPAN)
}

func GetNumReplicasServiceFlag() string {
	return getNumReplicasPrefix(SERVICE)
}

func GetNumReplicasDependenciesFlag() string {
	return getNumReplicasPrefix(DEPENDENCIES)
}

func GetNumReplicasSamplingFlag() string {
	return getNumReplicasPrefix(SAMPLING)
}

func getNumShardPrefix(indexType IndexType) string {
	return string(numShards + indexType)
}

func getNumReplicasPrefix(indexType IndexType) string {
	return string(numReplicas + indexType)
}

func GetPrioritySpanTemplate() string {
	return getPriorityTemplate(SPAN)
}

func GetPriorityServiceTemplate() string {
	return getPriorityTemplate(SERVICE)
}

func GetPriorityDependenciesTemplate() string {
	return getPriorityTemplate(DEPENDENCIES)
}

func GetPrioritySamplingTemplate() string {
	return getPriorityTemplate(SAMPLING)
}

func getPriorityTemplate(indexType IndexType) string {
	return fmt.Sprintf(priority, indexType)
}
