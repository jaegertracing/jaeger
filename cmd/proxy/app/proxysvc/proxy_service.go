// Copyright (c) 2021 The Jaeger Authors.
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

package proxysvc

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	errNoArchiveSpanStorage = errors.New("archive span storage was not configured")
)

const (
	defaultMaxClockSkewAdjust = time.Second
)

// ProxyServiceOptions has optional members of ProxyService
type ProxyServiceOptions struct {
	ArchiveSpanReader spanstore.Reader
	ArchiveSpanWriter spanstore.Writer
	Adjuster          adjuster.Adjuster
	Upstreams         []Upstream
}

type Upstream struct {
	GRPCHostPort string
	DialOptions  []grpc.DialOption
}

// ProxyService contains span utils required by the query-service.
type ProxyService struct {
	clients []*QueryClient
	logger  *zap.Logger
	options ProxyServiceOptions
}

// NewProxyService returns a new ProxyService.
func NewProxyService(options ProxyServiceOptions, logger *zap.Logger) *ProxyService {
	var clients []*QueryClient
	for _, upstream := range options.Upstreams {
		conn, err := grpc.DialContext(context.Background(), upstream.GRPCHostPort, upstream.DialOptions...)
		if err != nil {
			logger.Error("failed to connect to upstream", zap.String("upstream", upstream.GRPCHostPort), zap.Error(err))
			continue
		}
		clients = append(clients, NewQueryClient(conn))
	}
	qsvc := &ProxyService{
		clients: clients,
		logger:  logger,
		options: options,
	}

	if qsvc.options.Adjuster == nil {
		qsvc.options.Adjuster = adjuster.Sequence(StandardAdjusters(defaultMaxClockSkewAdjust, NewProcessTagAdjuster(nil, nil))...)
	}
	return qsvc
}

// GetTrace is the queryService implementation of spanstore.Reader.GetTrace
func (ps ProxyService) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	var wg sync.WaitGroup
	errChan := make(chan error)
	spansChan := make(chan []model.Span, 100)
	for _, client := range ps.clients {
		wg.Add(1)
		go func(client *QueryClient, wg *sync.WaitGroup) {
			defer wg.Done()
			spans, err := client.GetSpans(ctx, traceID)
			if err != nil {
				errChan <- err
				ps.logger.Error("failed to get trace from upstream", zap.Error(err))
				return
			}
			spansChan <- spans
		}(client, &wg)
	}
	wg.Wait()
	close(spansChan)
	close(errChan)

	var clientsErrors []error
	for err := range errChan {
		if err != nil {
			clientsErrors = append(clientsErrors, err)
		}
	}
	if len(clientsErrors) > 0 {
		return nil, multierror.Wrap(clientsErrors)
	}

	trace := model.Trace{}
	for spans := range spansChan {
		for i := range spans {
			trace.Spans = append(trace.Spans, &spans[i])
		}
	}

	return &trace, nil
}

// GetServices is the queryService implementation of spanstore.Reader.GetServices
func (ps ProxyService) GetServices(ctx context.Context) ([]string, error) {
	var wg sync.WaitGroup
	errChan := make(chan error)
	resultsChan := make(chan []string, 100)

	for _, client := range ps.clients {
		wg.Add(1)
		go func(client *QueryClient, wg *sync.WaitGroup) {
			defer wg.Done()
			res, err := client.GetServices(ctx)
			if err != nil {
				errChan <- err
				ps.logger.Error("failed to get services from upstream", zap.Error(err))
				return
			}
			resultsChan <- res
		}(client, &wg)
	}
	wg.Wait()
	close(resultsChan)
	close(errChan)

	var clientsErrors []error
	for err := range errChan {
		if err != nil {
			clientsErrors = append(clientsErrors, err)
		}
	}
	if len(clientsErrors) > 0 {
		return nil, multierror.Wrap(clientsErrors)
	}

	serviceMap := make(map[string]bool)
	for results := range resultsChan {
		for _, result := range results {
			serviceMap[result] = true
		}
	}
	var services []string
	for service := range serviceMap {
		services = append(services, service)
	}

	return services, nil
}

// GetOperations is the queryService implementation of spanstore.Reader.GetOperations
func (ps ProxyService) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	var wg sync.WaitGroup
	errChan := make(chan error)
	resultsChan := make(chan []spanstore.Operation, 100)

	for _, client := range ps.clients {
		wg.Add(1)
		go func(client *QueryClient, wg *sync.WaitGroup) {
			defer wg.Done()
			operations, err := client.GetOperations(ctx, query)
			if err != nil {
				errChan <- err
				ps.logger.Error("failed to get operations from upstream", zap.Error(err))
				return
			}
			resultsChan <- operations
		}(client, &wg)
	}
	wg.Wait()
	close(resultsChan)
	close(errChan)

	var clientsErrors []error
	for err := range errChan {
		if err != nil {
			clientsErrors = append(clientsErrors, err)
		}
	}
	if len(clientsErrors) > 0 {
		return nil, multierror.Wrap(clientsErrors)
	}

	var operations []spanstore.Operation
	for results := range resultsChan {
		operations = append(operations, results...)
	}

	return operations, nil
}

// FindTraces is the queryService implementation of spanstore.Reader.FindTraces
func (ps ProxyService) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	var wg sync.WaitGroup
	errChan := make(chan error)
	resultsChan := make(chan []*model.Trace, 100)

	for _, client := range ps.clients {
		wg.Add(1)
		go func(client *QueryClient, wg *sync.WaitGroup) {
			defer wg.Done()
			traces, err := client.FindTraces(ctx, query)
			if err != nil {
				errChan <- err
				ps.logger.Error("failed to find traces on upstream", zap.Error(err))
				return
			}
			resultsChan <- traces
		}(client, &wg)
	}
	wg.Wait()
	close(resultsChan)
	close(errChan)

	var clientsErrors []error
	for err := range errChan {
		if err != nil {
			clientsErrors = append(clientsErrors, err)
		}
	}
	if len(clientsErrors) > 0 {
		return nil, multierror.Wrap(clientsErrors)
	}

	var traces []*model.Trace
	for results := range resultsChan {
		traces = append(traces, results...)
	}

	return traces, nil
}

// ArchiveTrace is the queryService utility to archive traces.
func (ps ProxyService) ArchiveTrace(ctx context.Context, traceID model.TraceID) error {
	// TODO: should we proxy archive trace requests?
	return errNoArchiveSpanStorage
}

// Adjust applies adjusters to the trace.
func (ps ProxyService) Adjust(trace *model.Trace) (*model.Trace, error) {
	return ps.options.Adjuster.Adjust(trace)
}

// GetDependencies implements dependencystore.Reader.GetDependencies
func (ps ProxyService) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	var wg sync.WaitGroup
	errChan := make(chan error)
	resultsChan := make(chan []model.DependencyLink, 100)

	for _, client := range ps.clients {
		wg.Add(1)
		go func(client *QueryClient, wg *sync.WaitGroup) {
			defer wg.Done()
			traces, err := client.GetDependencies(ctx, endTs, lookback)
			if err != nil {
				errChan <- err
				ps.logger.Error("failed to get dependencies from upstream", zap.Error(err))
				return
			}
			resultsChan <- traces
		}(client, &wg)
	}
	wg.Wait()
	close(resultsChan)
	close(errChan)

	var clientsErrors []error
	for err := range errChan {
		if err != nil {
			clientsErrors = append(clientsErrors, err)
		}
	}
	if len(clientsErrors) > 0 {
		return nil, multierror.Wrap(clientsErrors)
	}

	var depLinks []model.DependencyLink
	for results := range resultsChan {
		depLinks = append(depLinks, results...)
	}

	return depLinks, nil
}
