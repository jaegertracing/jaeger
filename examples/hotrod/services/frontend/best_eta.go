// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/pool"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/config"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/customer"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/driver"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/route"
)

type bestETA struct {
	customer customer.Interface
	driver   driver.Interface
	route    route.Interface
	pool     *pool.Pool
	logger   log.Factory
}

// Response contains ETA for a trip.
type Response struct {
	Driver string
	ETA    time.Duration
}

func newBestETA(tracer trace.TracerProvider, logger log.Factory, options ConfigOptions) *bestETA {
	return &bestETA{
		customer: customer.NewClient(
			tracer,
			logger.With(zap.String("component", "customer_client")),
			options.CustomerHostPort,
		),
		driver: driver.NewClient(
			tracer,
			logger.With(zap.String("component", "driver_client")),
			options.DriverHostPort,
		),
		route: route.NewClient(
			tracer,
			logger.With(zap.String("component", "route_client")),
			options.RouteHostPort,
		),
		pool:   pool.New(config.RouteWorkerPoolSize),
		logger: logger,
	}
}

func (eta *bestETA) Get(ctx context.Context, customerID int) (*Response, error) {
	customer, err := eta.customer.Get(ctx, customerID)
	if err != nil {
		return nil, err
	}
	eta.logger.For(ctx).Info("Found customer", zap.Any("customer", customer))

	m, err := baggage.NewMember("customer", customer.Name)
	if err != nil {
		eta.logger.For(ctx).Error("cannot create baggage member", zap.Error(err))
	}
	bag := baggage.FromContext(ctx)
	bag, err = bag.SetMember(m)
	if err != nil {
		eta.logger.For(ctx).Error("cannot set baggage member", zap.Error(err))
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)

	drivers, err := eta.driver.FindNearest(ctx, customer.Location)
	if err != nil {
		return nil, err
	}
	eta.logger.For(ctx).Info("Found drivers", zap.Any("drivers", drivers))

	results := eta.getRoutes(ctx, customer, drivers)
	eta.logger.For(ctx).Info("Found routes", zap.Any("routes", results))

	resp := &Response{ETA: math.MaxInt64}
	for _, result := range results {
		if result.err != nil {
			return nil, err
		}
		if result.route.ETA < resp.ETA {
			resp.ETA = result.route.ETA
			resp.Driver = result.driver
		}
	}
	if resp.Driver == "" {
		return nil, errors.New("no routes found")
	}

	eta.logger.For(ctx).Info("Dispatch successful", zap.String("driver", resp.Driver), zap.String("eta", resp.ETA.String()))
	return resp, nil
}

type routeResult struct {
	driver string
	route  *route.Route
	err    error
}

// getRoutes calls Route service for each (customer, driver) pair
func (eta *bestETA) getRoutes(ctx context.Context, customer *customer.Customer, drivers []driver.Driver) []routeResult {
	results := make([]routeResult, 0, len(drivers))
	wg := sync.WaitGroup{}
	routesLock := sync.Mutex{}
	for _, dd := range drivers {
		wg.Add(1)
		driver := dd // capture loop var
		// Use worker pool to (potentially) execute requests in parallel
		eta.pool.Execute(func() {
			route, err := eta.route.FindRoute(ctx, driver.Location, customer.Location)
			routesLock.Lock()
			results = append(results, routeResult{
				driver: driver.DriverID,
				route:  route,
				err:    err,
			})
			routesLock.Unlock()
			wg.Done()
		})
	}
	wg.Wait()
	return results
}
