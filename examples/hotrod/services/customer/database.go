// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package customer

import (
	"context"
	"errors"

	"github.com/uber-go/zap"

	"github.com/opentracing/opentracing-go"
	tags "github.com/opentracing/opentracing-go/ext"

	"github.com/uber/jaeger/examples/hotrod/pkg/delay"
	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/pkg/tracing"
	"github.com/uber/jaeger/examples/hotrod/services/config"
)

// database simulates Customer repository implemented on top of an SQL database
type database struct {
	tracer    opentracing.Tracer
	logger    log.Factory
	customers map[string]*Customer
	lock      *tracing.Mutex
}

func newDatabase(tracer opentracing.Tracer, logger log.Factory) *database {
	return &database{
		tracer: tracer,
		logger: logger,
		lock: &tracing.Mutex{
			SessionBaggageKey: "session",
		},
		customers: map[string]*Customer{
			"123": {
				ID:       "123",
				Name:     "Rachel's Floral Designs",
				Location: "115,277",
			},
			"567": {
				ID:       "567",
				Name:     "Amazing Coffee Roasters",
				Location: "211,653",
			},
			"392": {
				ID:       "392",
				Name:     "Trom Chocolatier",
				Location: "577,322",
			},
			"731": {
				ID:       "731",
				Name:     "Japanese Deserts",
				Location: "728,326",
			},
		},
	}
}

func (d *database) Get(ctx context.Context, customerID string) (*Customer, error) {
	d.logger.For(ctx).Info("Loading customer", zap.String("customer_id", customerID))

	// simulate opentracing instrumentation of an SQL query
	if span := opentracing.SpanFromContext(ctx); span != nil {
		span := d.tracer.StartSpan("SQL SELECT", opentracing.ChildOf(span.Context()))
		tags.SpanKindRPCClient.Set(span)
		tags.PeerService.Set(span, "mysql")
		span.SetTag("sql.query", "SELECT * FROM customer WHERE customer_id="+customerID)
		defer span.Finish()
		ctx = opentracing.ContextWithSpan(ctx, span)
	}

	// simulate misconfigured connection pool that only gives one connection at a time
	d.lock.Lock(ctx)
	defer d.lock.Unlock()

	// simulate RPC delay
	delay.Sleep(config.MySQLGetDelay, config.MySQLGetDelayStdDev)

	if customer, ok := d.customers[customerID]; ok {
		return customer, nil
	}
	return nil, errors.New("invalid customer ID")
}
