package customer

import (
	"context"
	"errors"

	"github.com/uber-go/zap"

	"github.com/opentracing/opentracing-go"
	tags "github.com/opentracing/opentracing-go/ext"

	"code.uber.internal/infra/jaeger-demo/pkg/delay"
	"code.uber.internal/infra/jaeger-demo/pkg/log"
	"code.uber.internal/infra/jaeger-demo/pkg/tracing"
	"code.uber.internal/infra/jaeger-demo/services/config"
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
			"123": &Customer{
				ID:       "123",
				Name:     "Rachel's Floral Designs",
				Location: "115,277",
			},
			"567": &Customer{
				ID:       "567",
				Name:     "Amazing Coffee Roasters",
				Location: "211,653",
			},
			"392": &Customer{
				ID:       "392",
				Name:     "Trom Chocolatier",
				Location: "577,322",
			},
			"731": &Customer{
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
