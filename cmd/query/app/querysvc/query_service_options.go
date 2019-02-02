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

package querysvc

import (
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// QueryServiceOption is a function that sets some option on the QueryService
type QueryServiceOption func(qsvc *QueryService)

// QueryServiceOptions is a factory for all available QueryServiceOptions
var QueryServiceOptions queryServiceOptions

type queryServiceOptions struct{}

// Adjusters creates a QueryServiceOption that initializes the sequence of Adjusters on the QueryService.
func (queryServiceOptions) Adjusters(adjusters ...adjuster.Adjuster) QueryServiceOption {
	return func(queryService *QueryService) {
		queryService.adjuster = adjuster.Sequence(adjusters...)
	}
}

// ArchiveSpanReader creates a QueryServiceOption that initializes an ArchiveSpanReader on the QueryService.
func (queryServiceOptions) ArchiveSpanReader(reader spanstore.Reader) QueryServiceOption {
	return func(queryService *QueryService) {
		queryService.archiveSpanReader = reader
	}
}

// ArchiveSpanWriter creates a QueryServiceOption that initializes an ArchiveSpanWriter on the QueryService
func (queryServiceOptions) ArchiveSpanWriter(writer spanstore.Writer) QueryServiceOption {
	return func(queryService *QueryService) {
		queryService.archiveSpanWriter = writer
	}
}