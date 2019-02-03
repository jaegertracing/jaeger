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
	"errors"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	errNoArchiveSpanStorage = errors.New("archive span storage was not configured")
)

// QueryService contains span utils required by the query-service.
type QueryService struct {
	spanReader       spanstore.Reader
	dependencyReader dependencystore.Reader
	options          QueryServiceOptions
}

// NewQueryService returns a new QueryService.
func NewQueryService(spanReader spanstore.Reader, dependencyReader dependencystore.Reader, options QueryServiceOptions) *QueryService {
	qsvc := &QueryService{
		spanReader:       spanReader,
		dependencyReader: dependencyReader,
		options:          options,
	}

	if qsvc.options.adjuster == nil {
		qsvc.options.adjuster = adjuster.Sequence(StandardAdjusters...)
	}
	return qsvc
}

// Implement the spanstore.Reader interface

// GetTrace is the queryService implementation of spanstore.Reader.GetTrace
func (qs QueryService) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	trace, err := qs.spanReader.GetTrace(ctx, traceID)
	if err == spanstore.ErrTraceNotFound {
		if qs.options.archiveSpanReader == nil {
			return nil, err
		}
		trace, err = qs.options.archiveSpanReader.GetTrace(ctx, traceID)
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

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryService) ArchiveTrace(ctx context.Context, traceID *model.TraceID) error {
	if qs.options.archiveSpanReader == nil {
		return errNoArchiveSpanStorage
	}
	trace, err := qs.GetTrace(ctx, *traceID)
	if err != nil {
		return err
	}

	var writeErrors []error
	for _, span := range trace.Spans {
		err := qs.options.archiveSpanWriter.WriteSpan(span)
		if err != nil {
			writeErrors = append(writeErrors, err)
		}
	}
	multierr := multierror.Wrap(writeErrors)
	return multierr
}

// Implement the adjuster.Adjuster interface

// Adjust implements adjuster.Adjuster.Adjust
func (qs QueryService) Adjust(trace *model.Trace) (*model.Trace, error) {
	return qs.options.adjuster.Adjust(trace)
}

// Implement the dependencystore.Reader interface

// GetDependencies implements dependencystore.Reader.GetDependencies
func (qs QueryService) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return qs.dependencyReader.GetDependencies(endTs, lookback)
}
