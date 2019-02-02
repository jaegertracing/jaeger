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
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// QueryService contains span utils required by the query-service.
type QueryService struct {
	spanReader        spanstore.Reader
	archiveSpanReader spanstore.Reader
	archiveSpanWriter spanstore.Writer
	dependencyReader  dependencystore.Reader
	adjuster          adjuster.Adjuster
}

// NewQueryService returns a new QueryService.
func NewQueryService(spanReader spanstore.Reader, dependencyReader dependencystore.Reader, options ...QueryServiceOption) *QueryService {
	qsvc := &QueryService{
		spanReader:       spanReader,
		dependencyReader: dependencyReader,
	}

	for _, option := range options {
		option(qsvc)
	}
	if qsvc.adjuster == nil {
		qsvc.adjuster = adjuster.Sequence(StandardAdjusters...)
	}
	return qsvc
}

// Implement the spanstore.Reader interface

// GetTrace is the queryService implementation of spanstore.Reader.GetTrace
func (qs QueryService) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	trace, err := qs.spanReader.GetTrace(ctx, traceID)
	if err == spanstore.ErrTraceNotFound {
		if qs.archiveSpanReader == nil {
			return nil, err
		}
		trace, err = qs.archiveSpanReader.GetTrace(ctx, traceID)
	}
	return trace, err
}

// GetServices is the queryService implementation of spanstore.Reader.GetServices
func (qs QueryService) GetServices(ctx context.Context) ([]string, error) {
	return qs.spanReader.GetServices(ctx)
}

// GetOperations is the queryService implementation of spanstore.Reader.GetOperations
func (qs QueryService) GetOperations(ctx context.Context, service string) ([]string, error) {
	return qs.spanReader.GetOperations(ctx, service)
}

// FindTraces is the queryService implementation of spanstore.Reader.FindTraces
func (qs QueryService) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return qs.spanReader.FindTraces(ctx, query)
}

// FindTraceIDs is the queryService implementation of spanstore.Reader.FindTraceIDs
func (qs QueryService) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return qs.spanReader.FindTraceIDs(ctx, query)
}

// Implement the spanstore.Writer interface

// WriteSpan is the queryService implementation of spanstore.Writer.WriteSpan
func (qs QueryService) WriteSpan(span *model.Span) error {
	return qs.archiveSpanWriter.WriteSpan(span)
}

// CheckArchiveSpanWriter checks if archiveSpanWriter is nil.
func (qs QueryService) CheckArchiveSpanWriter() bool {
	return qs.archiveSpanWriter == nil
}


// Implement the adjuster.Adjuster interface

// Adjust implements adjuster.Adjuster.Adjust
func (qs QueryService) Adjust(trace *model.Trace) (*model.Trace, error) {
	return qs.adjuster.Adjust(trace)
}

// Implement the dependencystore.Reader interface

// GetDependencies implements dependencystore.Reader.GetDependencies
func (qs QueryService) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return qs.dependencyReader.GetDependencies(endTs, lookback)
}