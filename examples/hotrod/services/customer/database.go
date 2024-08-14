// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package customer

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/delay"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/config"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

// database simulates Customer repository implemented on top of an SQL database
type database struct {
	tracer    trace.Tracer
	logger    log.Factory
	customers map[int]*Customer
	lock      *tracing.Mutex
}

func newDatabase(tracer trace.Tracer, logger log.Factory) *database {
	return &database{
		tracer: tracer,
		logger: logger,
		lock: &tracing.Mutex{
			SessionBaggageKey: "request",
			LogFactory:        logger,
		},
		customers: map[int]*Customer{
			123: {
				ID:       "123",
				Name:     "Rachel's_Floral_Designs",
				Location: "115,277",
			},
			567: {
				ID:       "567",
				Name:     "Amazing_Coffee_Roasters",
				Location: "211,653",
			},
			392: {
				ID:       "392",
				Name:     "Trom_Chocolatier",
				Location: "577,322",
			},
			731: {
				ID:       "731",
				Name:     "Japanese_Desserts",
				Location: "728,326",
			},
		},
	}
}

func (d *database) Get(ctx context.Context, customerID int) (*Customer, error) {
	d.logger.For(ctx).Info("Loading customer", zap.Int("customer_id", customerID))

	ctx, span := d.tracer.Start(ctx, "SQL SELECT", trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(
		otelsemconv.PeerServiceKey.String("mysql"),
		attribute.
			Key("sql.query").
			String(fmt.Sprintf("SELECT * FROM customer WHERE customer_id=%d", customerID)),
	)
	defer span.End()

	if !config.MySQLMutexDisabled {
		// simulate misconfigured connection pool that only gives one connection at a time
		d.lock.Lock(ctx)
		defer d.lock.Unlock()
	}

	// simulate RPC delay
	delay.Sleep(config.MySQLGetDelay, config.MySQLGetDelayStdDev)

	if customer, ok := d.customers[customerID]; ok {
		return customer, nil
	}
	return nil, errors.New("invalid customer ID")
}
