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
	"fmt"
	"net/http"

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/pkg/tracing"
)

// Client is a remote client that implements customer.Interface
type Client struct {
	tracer opentracing.Tracer
	logger log.Factory
	client *tracing.HTTPClient
}

// NewClient creates a new customer.Client
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

// Get implements customer.Interface#Get as an RPC
func (c *Client) Get(ctx context.Context, customerID string) (*Customer, error) {
	c.logger.For(ctx).Info("Getting customer", zap.String("customer_id", customerID))

	url := fmt.Sprintf("http://127.0.0.1:8081/customer?customer=%s", customerID)
	var customer Customer
	if err := c.client.GetJSON(ctx, "/customer", url, &customer); err != nil {
		return nil, err
	}
	return &customer, nil
}
