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
	SPAN         IndexType = "spans"
	SERVICE      IndexType = "services"
	DEPENDENCIES IndexType = "dependencies"
	SAMPLING     IndexType = "sampling"

	numShards   = "num-shards-%s"
	numReplicas = "num-replicas-%s"
	priority    = "priority-%s-template"

	indexRolloverFrequency = "index-rollover-frequency-%s"
)

func NumShardSpanFlag() string {
	return numShardPrefix(SPAN)
}

func NumShardServiceFlag() string {
	return numShardPrefix(SERVICE)
}

func NumShardSamplingFlag() string {
	return numShardPrefix(SAMPLING)
}

func NumShardDependenciesFlag() string {
	return numShardPrefix(DEPENDENCIES)
}

func NumReplicaSpanFlag() string {
	return numReplicaPrefix(SPAN)
}

func NumReplicaServiceFlag() string {
	return numReplicaPrefix(SERVICE)
}

func NumReplicaDependenciesFlag() string {
	return numReplicaPrefix(DEPENDENCIES)
}

func NumReplicaSamplingFlag() string {
	return numReplicaPrefix(SAMPLING)
}

func numShardPrefix(indexType IndexType) string {
	return fmt.Sprintf(numShards, indexType)
}

func numReplicaPrefix(indexType IndexType) string {
	return fmt.Sprintf(numReplicas, indexType)
}

func PrioritySpanTemplateFlag() string {
	return priorityTemplate(SPAN)
}

func PriorityServiceTemplateFlag() string {
	return priorityTemplate(SERVICE)
}

func PriorityDependenciesTemplateFlag() string {
	return priorityTemplate(DEPENDENCIES)
}

func PrioritySamplingTemplateFlag() string {
	return priorityTemplate(SAMPLING)
}

func priorityTemplate(indexType IndexType) string {
	return fmt.Sprintf(priority, indexType)
}

func rolloverFrequency(indexType IndexType) string {
	return fmt.Sprintf(indexRolloverFrequency, indexType)
}

func IndexRolloverFrequencySpanFlag() string {
	return rolloverFrequency(SPAN)
}

func IndexRolloverFrequencyServiceFlag() string {
	return rolloverFrequency(SERVICE)
}

func IndexRolloverFrequencySamplingFlag() string {
	return rolloverFrequency(SAMPLING)
}

func IndexRolloverFrequencyDependenciesFlag() string {
	return rolloverFrequency(DEPENDENCIES)
}
