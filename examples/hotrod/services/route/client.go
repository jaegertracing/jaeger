// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"
	"net/url"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
)

// Client is a remote client that implements route.Interface
type Client struct {
	logger   log.Factory
	client   *tracing.HTTPClient
	hostPort string
}

// NewClient creates a new route.Client
func NewClient(tracer trace.TracerProvider, logger log.Factory, hostPort string) *Client {
	return &Client{
		logger:   logger,
		client:   tracing.NewHTTPClient(tracer),
		hostPort: hostPort,
	}
}

// FindRoute implements route.Interface#FindRoute as an RPC
func (c *Client) FindRoute(ctx context.Context, pickup, dropoff string) (*Route, error) {
	c.logger.For(ctx).Info("Finding route", zap.String("pickup", pickup), zap.String("dropoff", dropoff))

	v := url.Values{}
	v.Set("pickup", pickup)
	v.Set("dropoff", dropoff)
	url := "http://" + c.hostPort + "/route?" + v.Encode()
	var route Route
	if err := c.client.GetJSON(ctx, "/route", url, &route); err != nil {
		c.logger.For(ctx).Error("Error getting route", zap.Error(err))
		return nil, err
	}
	return &route, nil
}
