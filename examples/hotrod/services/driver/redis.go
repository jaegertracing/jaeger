// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/delay"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// Redis is a simulator of remote Redis cache
type Redis struct {
	tracer trace.Tracer // simulate redis as a separate process
	logger log.Factory
	errorSimulator
}

func newRedis(otelExporter string, metricsFactory metrics.Factory, logger log.Factory) *Redis {
	tp := tracing.InitOTEL("redis-manual", otelExporter, metricsFactory, logger)
	return &Redis{
		tracer: tp.Tracer("redis-manual"),
		logger: logger,
	}
}

// FindDriverIDs finds IDs of drivers who are near the location.
func (r *Redis) FindDriverIDs(ctx context.Context, location string) []string {
	ctx, span := r.tracer.Start(ctx, "FindDriverIDs", trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.Key("param.driver.location").String(location))
	defer span.End()

	// simulate RPC delay
	delay.Sleep(config.RedisFindDelay, config.RedisFindDelayStdDev)

	drivers := make([]string, 10)
	for i := range drivers {
		// #nosec
		drivers[i] = fmt.Sprintf("T7%05dC", rand.Int()%100000)
	}
	r.logger.For(ctx).Info("Found drivers", zap.Strings("drivers", drivers))
	return drivers
}

// GetDriver returns driver and the current car location
func (r *Redis) GetDriver(ctx context.Context, driverID string) (Driver, error) {
	ctx, span := r.tracer.Start(ctx, "GetDriver", trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.Key("param.driverID").String(driverID))
	defer span.End()

	// simulate RPC delay
	delay.Sleep(config.RedisGetDelay, config.RedisGetDelayStdDev)
	if err := r.checkError(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "An error occurred")
		r.logger.For(ctx).Error("redis timeout", zap.String("driver_id", driverID), zap.Error(err))
		return Driver{}, err
	}

	r.logger.For(ctx).Info("Got driver's ID", zap.String("driverID", driverID))

	// #nosec
	return Driver{
		DriverID: driverID,
		Location: fmt.Sprintf("%d,%d", rand.Int()%1000, rand.Int()%1000),
	}, nil
}

var errTimeout = errors.New("redis timeout")

type errorSimulator struct {
	sync.Mutex
	countTillError int
}

func (es *errorSimulator) checkError() error {
	es.Lock()
	es.countTillError--
	if es.countTillError > 0 {
		es.Unlock()
		return nil
	}
	es.countTillError = 5
	es.Unlock()
	delay.Sleep(2*config.RedisGetDelay, 0) // add more delay for "timeout"
	return errTimeout
}
