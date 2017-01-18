package route

import (
	"context"
	"net/http"
	"net/url"

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/zap"

	"code.uber.internal/infra/jaeger-demo/pkg/log"
	"code.uber.internal/infra/jaeger-demo/pkg/tracing"
)

// Client is a remote client that implements route.Interface
type Client struct {
	tracer opentracing.Tracer
	logger log.Factory
	client *tracing.HTTPClient
}

// NewClient creates a new route.Client
func NewClient(tracer opentracing.Tracer, logger log.Factory) *Client {
	return &Client{
		tracer: tracer,
		logger: logger,
		client: &tracing.HTTPClient{
			Client: &http.Client{Transport: &nethttp.Transport{}},
			Tracer: tracer,
		},
	}
}

// FindRoute implements route.Interface#FindRoute as an RPC
func (c *Client) FindRoute(ctx context.Context, pickup, dropoff string) (*Route, error) {
	c.logger.For(ctx).Info("Finding route", zap.String("pickup", pickup), zap.String("dropoff", dropoff))

	v := url.Values{}
	v.Set("pickup", pickup)
	v.Set("dropoff", dropoff)
	url := "http://127.0.0.1:8083/route?" + v.Encode()

	var route Route
	if err := c.client.GetJSON(ctx, "/route", url, &route); err != nil {
		c.logger.For(ctx).Error("Error getting route", zap.Error(err))
		return nil, err
	}
	return &route, nil
}
