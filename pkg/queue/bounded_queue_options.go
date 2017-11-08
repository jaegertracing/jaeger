// Copyright (c) 2017 Uber Technologies, Inc.
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

// Option is a function that sets some option on the BoundedQueue.
type Option func(c *Options)

// Options control behavior of the BoundedQueue.
type Options struct {
	priorityLevels int
	getPriority    func(item interface{}) int
}

// PriorityLevels determines the number of different priority levels for the bounded queue.
func PriorityLevels(priorityLevels int) Option {
	return func(o *Options) {
		o.priorityLevels = priorityLevels
	}
}

// GetPriority determines the priority level of a item.
func GetPriority(getPriority func(item interface{}) int) Option {
	return func(o *Options) {
		o.getPriority = getPriority
	}
}

func applyOptions(opts ...Option) Options {
	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.priorityLevels == 0 {
		o.priorityLevels = 1
	}
	if o.getPriority == nil {
		o.getPriority = func(item interface{}) int {
			return 0
		}
	}
	return o
}
