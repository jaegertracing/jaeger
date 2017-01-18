package route

import (
	"context"
	"expvar"
	"time"

	"github.com/opentracing/opentracing-go"
)

var routeCalcByCustomer = expvar.NewMap("route.calc.by.customer.sec")
var routeCalcBySession = expvar.NewMap("route.calc.by.session.sec")

var stats = []struct {
	expvar  *expvar.Map
	baggage string
}{
	{routeCalcByCustomer, "customer"},
	{routeCalcBySession, "session"},
}

func updateCalcStats(ctx context.Context, delay time.Duration) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return
	}
	delaySec := float64(delay/time.Millisecond) / 1000.0
	for _, s := range stats {
		key := span.BaggageItem(s.baggage)
		if key != "" {
			s.expvar.AddFloat(key, delaySec)
		}
	}
}
