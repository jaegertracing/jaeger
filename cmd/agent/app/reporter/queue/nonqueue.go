// Copyright (c) 2019 The Jaeger Authors.
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

package queue

import (
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

// NonQueue sends stuff directly without queueing. Useful for testing purposes
type NonQueue struct {
	processor func(*jaeger.Batch) error
}

// NewNonQueue returns direct processing "queue"
func NewNonQueue(processor func(*jaeger.Batch) error) *NonQueue {
	return &NonQueue{processor}
}

// Enqueue calls processor instead of queueing
func (n *NonQueue) Enqueue(batch *jaeger.Batch) error {
	return n.processor(batch)
}
