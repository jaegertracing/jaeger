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
	"github.com/jaegertracing/jaeger/model"
)

// NonQueue sends stuff directly without queueing. Useful for testing purposes
type NonQueue struct {
	processor func(model.Batch) (bool, error)
}

// NewNonQueue returns direct processing "queue"
func NewNonQueue(processor func(model.Batch) (bool, error)) *NonQueue {
	return &NonQueue{processor}
}

// Enqueue calls processor instead of queueing
func (n *NonQueue) Enqueue(batch model.Batch) error {
	_, err := n.processor(batch)
	return err
}

// Close implements io.Closer
func (n *NonQueue) Close() error {
	return nil
}
