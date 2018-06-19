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

// IndexFilter filters out any spans that should not be indexed.
type IndexFilter interface {
	IndexByDuration(span *Span) bool
	IndexByService(span *Span) bool
	IndexByOperation(span *Span) bool
}

// DefaultIndexFilter returns a filter that indexes everything.
var DefaultIndexFilter = indexFilterImpl{}

type indexFilterImpl struct{}

func (f indexFilterImpl) IndexByDuration(span *Span) bool {
	return true
}

func (f indexFilterImpl) IndexByService(span *Span) bool {
	return true
}

func (f indexFilterImpl) IndexByOperation(span *Span) bool {
	return true
}
