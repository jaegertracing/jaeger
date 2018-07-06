// Copyright (c) 2018 The Jaeger Authors.
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

package dbmodel

const (
	// DurationIndex represents the flag for indexing by duration.
	DurationIndex = iota

	// ServiceIndex represents the flag for indexing by service.
	ServiceIndex

	// OperationIndex represents the flag for indexing by service-operation.
	OperationIndex
)

// IndexFilter filters out any spans that should not be indexed depending on the index specified.
type IndexFilter func(span *Span, index int) bool

// DefaultIndexFilter is a filter that indexes everything.
var DefaultIndexFilter = func(span *Span, index int) bool {
	return true
}
