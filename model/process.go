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

package model

import "io"

// Process describes an instance of an application or service that emits tracing data.
type Process struct {
	ServiceName string    `json:"serviceName"`
	Tags        KeyValues `json:"tags,omitempty"`
}

// NewProcess creates a new Process for given serviceName and tags.
func NewProcess(serviceName string, tags []KeyValue) *Process {
	typedTags := KeyValues(tags)
	return &Process{ServiceName: serviceName, Tags: typedTags}
}

// Equal compares Process object with another Process.
func (p *Process) Equal(other *Process) bool {
	if p.ServiceName != other.ServiceName {
		return false
	}
	return p.Tags.Equal(other.Tags)
}

// Hash implements Hash from Hashable.
func (p *Process) Hash(w io.Writer) (err error) {
	if _, err := w.Write([]byte(p.ServiceName)); err != nil {
		return err
	}
	return p.Tags.Hash(w)
}
